// Package grafana is the ShopHub backend's Grafana admin-API client: it gives
// each tenant a dedicated Grafana Organization and a Viewer user scoped to it,
// so a tenant sees only their own Shop dashboards (spec 4.1 optional). It also
// holds the at-rest encryption used to store a tenant's Grafana password.
package grafana

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Provisioner talks to Grafana as the admin to create per-tenant orgs and users.
// Nil is never returned from methods; construct it via NewProvisionerFromEnv,
// which returns nil when GRAFANA_URL is unset (Grafana access disabled).
type Provisioner struct {
	apiURL string // in-cluster Grafana API base, e.g. http://kube-prometheus-stack-grafana.monitoring
	// ExternalURL is the URL the user opens in a browser, e.g. http://grafana.localhost:8080
	ExternalURL string
	user        string
	pass        string
	http        *http.Client
}

func NewProvisionerFromEnv() *Provisioner {
	api := envOr("GRAFANA_URL", "")
	if api == "" {
		return nil
	}
	return &Provisioner{
		apiURL:      api,
		ExternalURL: envOr("GRAFANA_EXTERNAL_URL", api),
		user:        envOr("GRAFANA_ADMIN_USER", "admin"),
		pass:        envOr("GRAFANA_ADMIN_PASSWORD", ""),
		http:        &http.Client{Timeout: 15 * time.Second},
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// EnsureUser ensures the tenant org exists and a Viewer user scoped to it. The
// password is set only when the user is created here; the caller stores it (so
// it can be shown to the tenant).
func (g *Provisioner) EnsureUser(ctx context.Context, namespace, email, password string) error {
	orgID, err := g.ensureOrg(ctx, namespace)
	if err != nil {
		return fmt.Errorf("ensure org: %w", err)
	}
	// A user created with OrgId lands in that org only (role Viewer), never in
	// the Main Org — verified against Grafana's admin API. So creating the user
	// is all that is needed for isolation.
	body := map[string]any{
		"name":     email,
		"email":    email,
		"login":    email,
		"password": password,
		"OrgId":    orgID,
	}
	err = g.do(ctx, http.MethodPost, "/api/admin/users", body, nil)
	if err == nil {
		return nil
	}
	// Already exists (e.g. a retried registration): make sure they are a Viewer
	// in the tenant org and move on.
	if apiErr, ok := err.(*grafanaAPIError); ok && (apiErr.Status == http.StatusPreconditionFailed || apiErr.Status == http.StatusConflict || apiErr.Status == http.StatusInternalServerError) {
		add := map[string]any{"loginOrEmail": email, "role": "Viewer"}
		if addErr := g.do(ctx, http.MethodPost, fmt.Sprintf("/api/orgs/%d/users", orgID), add, nil); addErr != nil {
			if a2, ok := addErr.(*grafanaAPIError); !ok || a2.Status != http.StatusConflict {
				return fmt.Errorf("add existing user to org: %w", addErr)
			}
		}
		return nil
	}
	return err
}

type grafanaOrgRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ensureOrg returns the id of the org named name, creating it if absent.
func (g *Provisioner) ensureOrg(ctx context.Context, name string) (int64, error) {
	var org grafanaOrgRef
	err := g.do(ctx, http.MethodGet, "/api/orgs/name/"+name, nil, &org)
	if err == nil {
		return org.ID, nil
	}
	if apiErr, ok := err.(*grafanaAPIError); !ok || apiErr.Status != http.StatusNotFound {
		return 0, err
	}
	var created struct {
		OrgID int64 `json:"orgId"`
	}
	if err := g.do(ctx, http.MethodPost, "/api/orgs", map[string]any{"name": name}, &created); err != nil {
		if err2 := g.do(ctx, http.MethodGet, "/api/orgs/name/"+name, nil, &org); err2 == nil {
			return org.ID, nil
		}
		return 0, err
	}
	return created.OrgID, nil
}

// grafanaAPIError is returned for any non-2xx Grafana response.
type grafanaAPIError struct {
	Status int
	Body   string
}

func (e *grafanaAPIError) Error() string {
	return fmt.Sprintf("grafana API %d: %s", e.Status, e.Body)
}

// do issues a Grafana API request authenticated as the Grafana admin.
func (g *Provisioner) do(ctx context.Context, method, path string, reqBody, respOut any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		buf, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.apiURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(g.user, g.pass)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &grafanaAPIError{Status: resp.StatusCode, Body: string(respBytes)}
	}
	if respOut != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, respOut); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// GeneratePassword returns a random hex password for a tenant's Grafana login.
func GeneratePassword() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
