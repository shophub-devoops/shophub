// Package main is the ShopHub backend: a thin REST API that creates and
// manages Shop CRs in the cluster on behalf of authenticated users.
// Authentication is intentionally absent in this skeleton (Faza 4 plan
// is JWT email/password; deferred). For now all requests target a single
// tenant namespace controlled by TENANT_NAMESPACE.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	if err := ensureUsersSchema(ctx, pool); err != nil {
		log.Fatalf("users schema: %v", err)
	}

	a := &auth{pool: pool, kube: kubeClient, secret: []byte(jwtSecret)}
	h := &handlers{kube: kubeClient}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger())
	r.Use(metricsMiddleware())

	r.GET("/probe/liveness", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/probe/readiness", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	api := r.Group("/api")
	{
		api.POST("/auth/register", a.register)
		api.POST("/auth/login", a.login)
		// Web3 wallet sign-in (spec 1.1 optional): nonce -> sign in MetaMask -> verify.
		api.POST("/auth/nonce", a.nonce)
		api.POST("/auth/wallet", a.walletLogin)

		// Shop management requires a valid JWT; each request is scoped to the
		// caller's tenant namespace (set by the auth middleware).
		shops := api.Group("/shops", a.middleware())
		shops.GET("", h.listShops)
		shops.POST("", h.createShop)
		shops.GET("/:name", h.getShop)
		shops.PUT("/:name", h.updateShop)
		shops.DELETE("/:name", h.deleteShop)
	}

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

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("%s %s -> %d (%s)", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start))
	}
}
