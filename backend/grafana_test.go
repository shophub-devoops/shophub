package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// fakeGrafana records org/user provisioning so we can assert ensureUser creates
// the org and a user scoped to it.
type fakeGrafana struct {
	mu          sync.Mutex
	orgsByName  map[string]int64
	nextOrgID   int64
	createdUser map[string]int64 // login -> orgID it was created under
}

func newFakeGrafana() *fakeGrafana {
	return &fakeGrafana{orgsByName: map[string]int64{}, nextOrgID: 1, createdUser: map[string]int64{}}
}

func (f *fakeGrafana) server() *httptest.Server {
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

	// Org-membership add (used on the "already exists" path).
	mux.HandleFunc("/api/orgs/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return httptest.NewServer(mux)
}

func TestEnsureUserCreatesScopedAccount(t *testing.T) {
	fake := newFakeGrafana()
	srv := fake.server()
	defer srv.Close()

	g := &grafanaProvisioner{apiURL: srv.URL, user: "admin", pass: "pw", http: srv.Client()}
	if err := g.ensureUser(context.Background(), "tenant-1", "a@b.c", "secret"); err != nil {
		t.Fatalf("ensureUser: %v", err)
	}

	orgID := fake.orgsByName["tenant-1"]
	if orgID == 0 {
		t.Fatal("tenant org not created")
	}
	if got := fake.createdUser["a@b.c"]; got != orgID {
		t.Errorf("user created under org %d, want tenant org %d", got, orgID)
	}
}

// TestGrafanaInfoReprovisionsAfterWipe guards the ephemeral-Grafana case: a
// Grafana restart wipes orgs and users, so /api/grafana must recreate the
// tenant's account (with the stored password) instead of returning credentials
// for an account that no longer exists.
func TestGrafanaInfoReprovisionsAfterWipe(t *testing.T) {
	fake := newFakeGrafana()
	srv := fake.server()
	defer srv.Close()

	a := newTestAuth(t)
	a.grafana = &grafanaProvisioner{apiURL: srv.URL, externalURL: "http://grafana.example", user: "admin", pass: "pw", http: srv.Client()}

	r := gin.New()
	r.POST("/api/auth/register", a.register)
	r.GET("/api/grafana", a.middleware(), a.grafanaInfo)

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
	fake.mu.Lock()
	fake.orgsByName = map[string]int64{}
	fake.createdUser = map[string]int64{}
	fake.mu.Unlock()

	w = do(r, http.MethodGet, "/api/grafana", reg.Token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("grafanaInfo after wipe = %d (body: %s)", w.Code, w.Body.String())
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.createdUser) != 1 {
		t.Errorf("user not recreated after wipe: %+v", fake.createdUser)
	}
	if len(fake.orgsByName) != 1 {
		t.Errorf("org not recreated after wipe: %+v", fake.orgsByName)
	}
}

// TestGrafanaPasswordEncryptionRoundTrip guards the at-rest encryption (the
// stored Grafana password must round-trip but never sit in the DB as plaintext,
// and must not decrypt under a different key).
func TestGrafanaPasswordEncryptionRoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	const pw = "s3cr3t-grafana-pw"

	enc, err := encryptGrafanaPassword(secret, pw)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if strings.Contains(enc, pw) {
		t.Fatalf("ciphertext leaks plaintext: %q", enc)
	}

	got, err := decryptGrafanaPassword(secret, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != pw {
		t.Fatalf("round-trip = %q, want %q", got, pw)
	}

	if _, err := decryptGrafanaPassword([]byte("other-secret"), enc); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}

func TestEnsureUserIdempotentOnExisting(t *testing.T) {
	fake := newFakeGrafana()
	srv := fake.server()
	defer srv.Close()

	g := &grafanaProvisioner{apiURL: srv.URL, user: "admin", pass: "pw", http: srv.Client()}
	ctx := context.Background()
	if err := g.ensureUser(ctx, "tenant-1", "a@b.c", "secret"); err != nil {
		t.Fatalf("first ensureUser: %v", err)
	}
	// Second call hits the "already exists" branch and must not error (the
	// add-to-org fallback returns 200 from the fake).
	if err := g.ensureUser(ctx, "tenant-1", "a@b.c", "secret"); err != nil {
		t.Fatalf("second ensureUser should be idempotent, got: %v", err)
	}
}
