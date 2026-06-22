package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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

	"github.com/shophub-devoops/shophub/backend/internal/grafana"
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
	if err := EnsureUsersSchema(ctx, testPool); err != nil {
		log.Fatalf("schema: %v", err)
	}

	code := m.Run()

	testPool.Close()
	_ = pg.Terminate(ctx)
	os.Exit(code)
}

// newTestAuth wires an Auth against the shared Postgres and a fresh in-memory
// fake kube client (so register can create tenant namespaces without a cluster).
func newTestAuth(t *testing.T) *Auth {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	return &Auth{
		Pool:   testPool,
		Kube:   fake.NewClientBuilder().WithScheme(scheme).Build(),
		Secret: []byte("test-secret"),
	}
}

// router builds the HTTP surface under test: public auth routes plus a
// middleware-protected probe that echoes the caller's namespace.
func router(a *Auth) *gin.Engine {
	r := gin.New()
	api := r.Group("/api")
	api.POST("/auth/register", a.Register)
	api.POST("/auth/login", a.Login)
	api.GET("/whoami", a.Middleware(), func(c *gin.Context) {
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

	if w := do(r, http.MethodGet, "/api/whoami", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no-token = %d, want 401", w.Code)
	}

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

// --- Grafana reprovision integration test -------------------------------------

// fakeGrafanaState records org/user provisioning for the reprovision test.
type fakeGrafanaState struct {
	mu          sync.Mutex
	orgsByName  map[string]int64
	nextOrgID   int64
	createdUser map[string]int64
}

func fakeGrafanaServer(f *fakeGrafanaState) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/name/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		name := strings.TrimPrefix(r.URL.Path, "/api/orgs/name/")
		id, ok := f.orgsByName[name]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "name": name})
	})
	mux.HandleFunc("/api/orgs", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := f.nextOrgID
		f.nextOrgID++
		f.orgsByName[body.Name] = id
		_ = json.NewEncoder(w).Encode(map[string]any{"orgId": id})
	})
	mux.HandleFunc("/api/admin/users", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body struct {
			Login string `json:"login"`
			OrgId int64  `json:"OrgId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, exists := f.createdUser[body.Login]; exists {
			http.Error(w, "user already exists", http.StatusPreconditionFailed)
			return
		}
		f.createdUser[body.Login] = body.OrgId
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 100})
	})
	mux.HandleFunc("/api/orgs/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return httptest.NewServer(mux)
}

// TestGrafanaInfoReprovisionsAfterWipe guards the ephemeral-Grafana case: a
// Grafana restart wipes orgs and users, so /api/grafana must recreate the
// tenant's account (with the stored, encrypted password) instead of returning
// credentials for an account that no longer exists.
func TestGrafanaInfoReprovisionsAfterWipe(t *testing.T) {
	f := &fakeGrafanaState{orgsByName: map[string]int64{}, nextOrgID: 1, createdUser: map[string]int64{}}
	srv := fakeGrafanaServer(f)
	defer srv.Close()

	t.Setenv("GRAFANA_URL", srv.URL)
	t.Setenv("GRAFANA_EXTERNAL_URL", "http://grafana.example")
	t.Setenv("GRAFANA_ADMIN_PASSWORD", "pw")

	a := newTestAuth(t)
	a.Grafana = grafana.NewProvisionerFromEnv()

	r := gin.New()
	r.POST("/api/auth/register", a.Register)
	r.GET("/api/grafana", a.Middleware(), a.GrafanaInfo)

	// Registration provisions the account.
	w := do(r, http.MethodPost, "/api/auth/register", "", creds("wipe@test.local"))
	if w.Code != http.StatusCreated {
		t.Fatalf("register = %d (body: %s)", w.Code, w.Body.String())
	}
	var reg struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reg); err != nil {
		t.Fatalf("decode register: %v", err)
	}

	// Simulate a Grafana restart wiping its ephemeral database.
	f.mu.Lock()
	f.orgsByName = map[string]int64{}
	f.createdUser = map[string]int64{}
	f.mu.Unlock()

	w = do(r, http.MethodGet, "/api/grafana", reg.Token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("grafanaInfo after wipe = %d (body: %s)", w.Code, w.Body.String())
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.createdUser) != 1 {
		t.Errorf("user not recreated after wipe: %+v", f.createdUser)
	}
	if len(f.orgsByName) != 1 {
		t.Errorf("org not recreated after wipe: %+v", f.orgsByName)
	}
}
