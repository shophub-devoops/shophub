// Command shophub is the ShopHub backend: a REST API that creates and manages
// Shop CRs in the cluster on behalf of authenticated users. Users authenticate
// with email/password (or a Web3 wallet signature) and receive a JWT; each user
// is scoped to their own tenant namespace, embedded in the token, so every Shop
// operation is isolated per user.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "github.com/shophub-devoops/shop-operator/api/apps/v1"
	notifyv1 "github.com/shophub-devoops/shop-operator/api/notify/v1"
	paymentsv1 "github.com/shophub-devoops/shop-operator/api/payments/v1"

	"github.com/shophub-devoops/shophub/backend/internal/auth"
	"github.com/shophub-devoops/shophub/backend/internal/grafana"
	"github.com/shophub-devoops/shophub/backend/internal/httpapi"
	"github.com/shophub-devoops/shophub/backend/internal/observability"
)

func main() {
	addr := getenv("LISTEN_ADDR", ":8080")
	databaseURL := os.Getenv("DATABASE_URL")
	jwtSecret := os.Getenv("JWT_SECRET")
	if databaseURL == "" || jwtSecret == "" {
		log.Fatal("DATABASE_URL and JWT_SECRET are required")
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(notifyv1.AddToScheme(scheme))
	utilruntime.Must(paymentsv1.AddToScheme(scheme))

	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Fatalf("kubeconfig: %v", err)
	}
	kubeClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("k8s client: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("db pool: %v", err)
	}
	defer pool.Close()
	if err := auth.EnsureUsersSchema(ctx, pool); err != nil {
		log.Fatalf("users schema: %v", err)
	}

	a := &auth.Auth{Pool: pool, Kube: kubeClient, Secret: []byte(jwtSecret), Grafana: grafana.NewProvisionerFromEnv()}
	h := &httpapi.Handlers{
		Kube: kubeClient,
		// Platform-level Discord setup (one bot + one guild, configured by the
		// chart). Empty guild disables the per-shop Discord channel option.
		Discord: httpapi.DiscordConfig{
			GuildID:       os.Getenv("DISCORD_GUILD_ID"),
			BotSecretName: os.Getenv("DISCORD_BOT_TOKEN_SECRET_NAME"),
			BotSecretNS:   os.Getenv("DISCORD_BOT_TOKEN_SECRET_NAMESPACE"),
		},
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(observability.RequestLogger())
	r.Use(observability.Middleware())

	r.GET("/probe/liveness", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/probe/readiness", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Per-IP limit on the unauthenticated auth endpoints. Registration creates a
	// tenant namespace per call, so it must not be free to spam.
	authLimit := auth.NewRateLimit(0.1, 5) // 5 quick requests, then one per 10s

	api := r.Group("/api")
	{
		api.POST("/auth/register", authLimit.Middleware(), a.Register)
		api.POST("/auth/login", authLimit.Middleware(), a.Login)
		// Web3 wallet sign-in (spec 1.1 optional): nonce -> sign in MetaMask -> verify.
		api.POST("/auth/nonce", authLimit.Middleware(), a.Nonce)
		api.POST("/auth/wallet", authLimit.Middleware(), a.WalletLogin)

		// Shop management requires a valid JWT; each request is scoped to the
		// caller's tenant namespace (set by the auth middleware).
		shops := api.Group("/shops", a.Middleware())
		shops.GET("", h.ListShops)
		shops.POST("", h.CreateShop)
		shops.GET("/:name", h.GetShop)
		shops.PUT("/:name", h.UpdateShop)
		shops.DELETE("/:name", h.DeleteShop)
		// Admin credentials for the shop's own dashboard (operator-generated).
		shops.GET("/:name/admin-credentials", h.GetShopAdminCredentials)

		// Wallet generation via the operator's Wallet CRD: creates a keypair on
		// the tenant's behalf and returns the public address.
		api.POST("/wallets", a.Middleware(), h.CreateWallet)

		// Per-tenant Grafana access: link + scoped login for the caller's org.
		api.GET("/grafana", a.Middleware(), a.GrafanaInfo)
	}

	// Serve the built ShopHub SPA (bundled into the unified image) so the
	// platform is reachable at "/" from the cluster ingress, not just the API.
	mountFrontend(r)

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("shophub backend listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// mountFrontend serves the built frontend SPA from WEB_DIR (populated in the
// unified container image). No-op when no bundled UI is present (local API-only
// runs / tests). Non-API GETs fall back to index.html so client-side routes
// (/login, /dashboard) work on refresh.
func mountFrontend(r *gin.Engine) {
	webDir := getenv("WEB_DIR", "/app/web")
	index := filepath.Join(webDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		return
	}
	r.Static("/assets", filepath.Join(webDir, "assets"))
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if c.Request.Method != http.MethodGet ||
			strings.HasPrefix(p, "/api") || strings.HasPrefix(p, "/metrics") || strings.HasPrefix(p, "/probe") {
			c.Status(http.StatusNotFound)
			return
		}
		c.File(index)
	})
}
