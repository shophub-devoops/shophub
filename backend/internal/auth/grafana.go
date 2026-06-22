package auth

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/shophub-devoops/shophub/backend/internal/grafana"
)

// provisionGrafana creates the tenant's Grafana login and stores its generated
// password (encrypted at rest) so GrafanaInfo can hand it back to the tenant.
// Best-effort: any failure is logged, never surfaced to the caller, so
// registration still succeeds when Grafana is unavailable.
func (a *Auth) provisionGrafana(ctx context.Context, ns, email string) {
	if a.Grafana == nil {
		return
	}
	password, err := grafana.GeneratePassword()
	if err != nil {
		log.Printf("grafana: generate password: %v", err)
		return
	}
	if err := a.Grafana.EnsureUser(ctx, ns, email, password); err != nil {
		log.Printf("grafana: provision user for %s: %v", ns, err)
		return
	}
	enc, err := grafana.EncryptPassword(a.Secret, password)
	if err != nil {
		log.Printf("grafana: encrypt password for %s: %v", ns, err)
		return
	}
	if _, err := a.Pool.Exec(ctx,
		`UPDATE users SET grafana_login = $1, grafana_password = $2 WHERE namespace = $3`,
		email, enc, ns); err != nil {
		log.Printf("grafana: store creds for %s: %v", ns, err)
	}
}

// GrafanaInfo returns the logged-in tenant's Grafana access details (link +
// credentials) so the UI can offer a "view my metrics" button. The password was
// generated and stored at registration; the account itself is re-asserted in
// Grafana on every call because Grafana's database is ephemeral (a restart wipes
// orgs and users), so we lazily re-provision instead of handing out credentials
// for an account that no longer exists.
func (a *Auth) GrafanaInfo(c *gin.Context) {
	if a.Grafana == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "grafana access is not configured"})
		return
	}
	ns := nsFromCtx(c)
	var email, login, password string
	err := a.Pool.QueryRow(c.Request.Context(),
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
		err = a.Pool.QueryRow(c.Request.Context(),
			`SELECT grafana_login, grafana_password FROM users WHERE namespace = $1`, ns).Scan(&login, &password)
		if err != nil || login == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "grafana account could not be provisioned — try again later"})
			return
		}
	}

	// The stored password is encrypted at rest; decrypt before using it.
	plain, err := grafana.DecryptPassword(a.Secret, password)
	if err != nil {
		log.Printf("grafana: decrypt password for %s: %v", ns, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "grafana account needs re-provisioning — try again later"})
		return
	}

	// Re-assert the account with the stored password (idempotent; recreates it
	// after a Grafana restart). Failure here means the credentials we'd return
	// may not work, so surface it instead of handing them out.
	if err := a.Grafana.EnsureUser(c.Request.Context(), ns, login, plain); err != nil {
		log.Printf("grafana: re-provision user for %s: %v", ns, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "grafana is unavailable — try again later"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":      a.Grafana.ExternalURL,
		"login":    login,
		"password": plain,
		"org":      ns,
	})
}
