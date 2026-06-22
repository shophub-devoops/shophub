package grafana

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeGrafana records org/user provisioning so we can assert EnsureUser creates
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

	g := &Provisioner{apiURL: srv.URL, user: "admin", pass: "pw", http: srv.Client()}
	if err := g.EnsureUser(context.Background(), "tenant-1", "a@b.c", "secret"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}

	orgID := fake.orgsByName["tenant-1"]
	if orgID == 0 {
		t.Fatal("tenant org not created")
	}
	if got := fake.createdUser["a@b.c"]; got != orgID {
		t.Errorf("user created under org %d, want tenant org %d", got, orgID)
	}
}

func TestEnsureUserIdempotentOnExisting(t *testing.T) {
	fake := newFakeGrafana()
	srv := fake.server()
	defer srv.Close()

	g := &Provisioner{apiURL: srv.URL, user: "admin", pass: "pw", http: srv.Client()}
	ctx := context.Background()
	if err := g.EnsureUser(ctx, "tenant-1", "a@b.c", "secret"); err != nil {
		t.Fatalf("first EnsureUser: %v", err)
	}
	// Second call hits the "already exists" branch and must not error (the
	// add-to-org fallback returns 200 from the fake).
	if err := g.EnsureUser(ctx, "tenant-1", "a@b.c", "secret"); err != nil {
		t.Fatalf("second EnsureUser should be idempotent, got: %v", err)
	}
}
