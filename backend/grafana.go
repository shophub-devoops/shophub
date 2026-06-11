package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// grafanaProvisioner gives each ShopHub tenant a dedicated Grafana Organization
// and a Viewer user scoped to it, so a tenant can open Grafana and see only
// their own Shop dashboards (spec 4.1 optional). The Shop operator fills that
// org with the tenant's dashboards + datasources; this side creates the org and
// the login. In OSS Grafana the basic Viewer role is org-wide, so a separate org
// per tenant is the only way to keep tenants from seeing each other's data.
//
// Nil when GRAFANA_URL is unset — registration then skips Grafana provisioning.
type grafanaProvisioner struct {
	apiURL      string // in-cluster Grafana API base, e.g. http://kube-prometheus-stack-grafana.monitoring
	externalURL string // URL the user opens in a browser, e.g. http://grafana.localhost:8080
	user        string
	pass        string
	http        *http.Client
}

func newGrafanaProvisionerFromEnv() *grafanaProvisioner {
	api := getenv("GRAFANA_URL", "")
	if api == "" {
		return nil
	}
	return &grafanaProvisioner{
		apiURL:      api,
		externalURL: getenv("GRAFANA_EXTERNAL_URL", api),
		user:        getenv("GRAFANA_ADMIN_USER", "admin"),
		pass:        getenv("GRAFANA_ADMIN_PASSWORD", ""),
		http:        &http.Client{Timeout: 15 * time.Second},
	}
}

// provisionGrafana creates the tenant's Grafana login and stores its generated
// password so grafanaInfo can hand it back to the tenant. Best-effort: any
// failure is logged, never surfaced to the caller, so registration still
// succeeds when Grafana is unavailable.
func (a *auth) provisionGrafana(ctx context.Context, ns, email string) {
	if a.grafana == nil {
		return
	}
	password, err := generatePassword()
	if err != nil {
		log.Printf("grafana: generate password: %v", err)
		return
	}
	if err := a.grafana.ensureUser(ctx, ns, email, password); err != nil {
		log.Printf("grafana: provision user for %s: %v", ns, err)
		return
	}
	if _, err := a.pool.Exec(ctx,
		`UPDATE users SET grafana_login = $1, grafana_password = $2 WHERE namespace = $3`,
		email, password, ns); err != nil {
		log.Printf("grafana: store creds for %s: %v", ns, err)
	}
}

// ensureUser ensures the tenant org exists and a Viewer user scoped to it. The
// returned password is stored by the caller so it can be shown to the tenant;
// it is set only when the user is created here.
func (g *grafanaProvisioner) ensureUser(ctx context.Context, namespace, email, password string) error {
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
func (g *grafanaProvisioner) ensureOrg(ctx context.Context, name string) (int64, error) {
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

// grafanaInfo returns the logged-in tenant's Grafana access details (link +
// credentials) so the UI can offer a "view my metrics" button. The password was
// generated and stored at registration; the account itself is re-asserted in
// Grafana on every call because Grafana's database is ephemeral (no
// persistence) — a Grafana restart wipes orgs and users, so we lazily
// re-provision instead of handing out credentials for an account that no
// longer exists.
func (a *auth) grafanaInfo(c *gin.Context) {
	if a.grafana == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "grafana access is not configured"})
		return
	}
	ns := nsFromCtx(c)
	var email, login, password string
	err := a.pool.QueryRow(c.Request.Context(),
		`SELECT email, grafana_login, grafana_password FROM users WHERE namespace = $1`, ns).Scan(&email, &login, &password)
	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "no grafana account for this tenant"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup grafana creds: " + err.Error()})
		return
	}

	if login == "" {
		// Never provisioned (e.g. Grafana was down at registration) — do it now.
		a.provisionGrafana(c.Request.Context(), ns, email)
		err = a.pool.QueryRow(c.Request.Context(),
			`SELECT grafana_login, grafana_password FROM users WHERE namespace = $1`, ns).Scan(&login, &password)
		if err != nil || login == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "grafana account could not be provisioned — try again later"})
			return
		}
	} else if err := a.grafana.ensureUser(c.Request.Context(), ns, login, password); err != nil {
		// Re-assert the account with the stored password (idempotent; recreates
		// it after a Grafana restart). Failure here means the credentials we'd
		// return may not work, so surface it instead of handing them out.
		log.Printf("grafana: re-provision user for %s: %v", ns, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "grafana is unavailable — try again later"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":      a.grafana.externalURL,
		"login":    login,
		"password": password,
		"org":      ns,
	})
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
func (g *grafanaProvisioner) do(ctx context.Context, method, path string, reqBody, respOut any) error {
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

// generatePassword returns a random hex password for a tenant's Grafana login.
func generatePassword() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
