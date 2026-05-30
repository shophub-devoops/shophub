package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// testPool is a connection pool to a throwaway Postgres started once for the
// whole package (Testcontainers — spec 5.2 integration-test infrastructure).
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()

	pg, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("shophub"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("connection string: %v", err)
	}
	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("pool: %v", err)
	}
	if err := ensureUsersSchema(ctx, testPool); err != nil {
		log.Fatalf("schema: %v", err)
	}

	code := m.Run()

	testPool.Close()
	_ = pg.Terminate(ctx)
	os.Exit(code)
}

// newTestAuth wires an auth against the shared Postgres and a fresh in-memory
// fake kube client (so register can create tenant namespaces without a cluster).
func newTestAuth(t *testing.T) *auth {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	return &auth{
		pool:   testPool,
		kube:   fake.NewClientBuilder().WithScheme(scheme).Build(),
		secret: []byte("test-secret"),
	}
}

// router builds the HTTP surface under test: public auth routes plus a
// middleware-protected probe that echoes the caller's namespace.
func router(a *auth) *gin.Engine {
	r := gin.New()
	api := r.Group("/api")
	api.POST("/auth/register", a.register)
	api.POST("/auth/login", a.login)
	api.GET("/whoami", a.middleware(), func(c *gin.Context) {
		c.String(http.StatusOK, nsFromCtx(c))
	})
	return r
}

func do(r *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func creds(email string) map[string]string {
	return map[string]string{"email": email, "password": "hunter2pass"}
}

func TestRegisterReturnsTokenAndNamespace(t *testing.T) {
	r := router(newTestAuth(t))
	w := do(r, http.MethodPost, "/api/auth/register", "", creds("alice@example.com"))

	if w.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want 201 (body: %s)", w.Code, w.Body.String())
	}
	var resp struct{ Token, Namespace string }
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" || resp.Namespace == "" {
		t.Fatalf("expected token and namespace, got %+v", resp)
	}
}

func TestRegisterDuplicateEmailConflicts(t *testing.T) {
	r := router(newTestAuth(t))
	_ = do(r, http.MethodPost, "/api/auth/register", "", creds("dup@example.com"))
	w := do(r, http.MethodPost, "/api/auth/register", "", creds("dup@example.com"))
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate register = %d, want 409 (body: %s)", w.Code, w.Body.String())
	}
}

func TestLoginSucceedsWithCorrectPassword(t *testing.T) {
	r := router(newTestAuth(t))
	email := "login-ok@example.com"
	if w := do(r, http.MethodPost, "/api/auth/register", "", creds(email)); w.Code != http.StatusCreated {
		t.Fatalf("setup register failed: %d", w.Code)
	}
	w := do(r, http.MethodPost, "/api/auth/login", "", creds(email))
	if w.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	r := router(newTestAuth(t))
	email := "login-bad@example.com"
	_ = do(r, http.MethodPost, "/api/auth/register", "", creds(email))
	w := do(r, http.MethodPost, "/api/auth/login", "",
		map[string]string{"email": email, "password": "wrongpassword"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-password login = %d, want 401", w.Code)
	}
}

func TestLoginRejectsUnknownUser(t *testing.T) {
	r := router(newTestAuth(t))
	w := do(r, http.MethodPost, "/api/auth/login", "", creds("ghost@example.com"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown-user login = %d, want 401", w.Code)
	}
}

func TestProtectedRouteRequiresValidToken(t *testing.T) {
	a := newTestAuth(t)
	r := router(a)

	// No token → 401.
	if w := do(r, http.MethodGet, "/api/whoami", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no-token = %d, want 401", w.Code)
	}

	// Register, then use the returned token → 200 echoing the tenant namespace.
	reg := do(r, http.MethodPost, "/api/auth/register", "", creds("whoami@example.com"))
	var resp struct{ Token, Namespace string }
	_ = json.Unmarshal(reg.Body.Bytes(), &resp)

	w := do(r, http.MethodGet, "/api/whoami", resp.Token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("valid-token = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != resp.Namespace {
		t.Fatalf("whoami namespace = %q, want %q", got, resp.Namespace)
	}
}
