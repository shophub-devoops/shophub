package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const tokenTTL = 24 * time.Hour

// auth carries auth dependencies: the user store (Postgres), the kube client
// (to create each user's tenant namespace), the JWT signing secret, and an
// optional Grafana provisioner (per-tenant org + scoped login).
type auth struct {
	pool    *pgxpool.Pool
	kube    client.Client
	secret  []byte
	grafana *grafanaProvisioner
}

// ensureUsersSchema creates the users table on startup. Each user owns one
// tenant namespace where their Shop CRs live. The grafana_* columns hold the
// per-tenant Grafana login provisioned at registration (empty when Grafana
// access is not configured).
func ensureUsersSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id            bigserial PRIMARY KEY,
			email         text UNIQUE NOT NULL,
			password_hash text NOT NULL,
			namespace     text UNIQUE NOT NULL,
			created_at    timestamptz NOT NULL DEFAULT now()
		);
		ALTER TABLE users ADD COLUMN IF NOT EXISTS grafana_login    text NOT NULL DEFAULT '';
		ALTER TABLE users ADD COLUMN IF NOT EXISTS grafana_password text NOT NULL DEFAULT '';`)
	return err
}

type credentials struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// claims embeds the user's tenant namespace so the Shop handlers can scope
// every operation to the caller without another DB lookup.
type claims struct {
	Namespace string `json:"ns"`
	jwt.RegisteredClaims
}

// register creates a user and their own tenant namespace (full per-user
// isolation), then returns a JWT so the client is logged in immediately.
func (a *auth) register(c *gin.Context) {
	var in credentials
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash password"})
		return
	}

	ns, err := randomNamespace()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate namespace"})
		return
	}

	// Insert the user first so a duplicate email fails cleanly, before we
	// create any cluster resources.
	_, err = a.pool.Exec(c.Request.Context(),
		`INSERT INTO users (email, password_hash, namespace) VALUES ($1, $2, $3)`,
		email, string(hash), ns)
	if isUniqueViolation(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create user: " + err.Error()})
		return
	}

	// Materialize the tenant namespace (idempotent). On failure the user row
	// already exists; login re-runs ensureNamespace, so the account self-heals.
	if err := a.ensureNamespace(c.Request.Context(), ns); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create namespace: " + err.Error()})
		return
	}

	// Provision a per-tenant Grafana org + Viewer login so the user can view
	// only their own dashboards (spec 4.1 optional). Best-effort: a Grafana
	// hiccup must not fail registration — the account can be re-provisioned.
	a.provisionGrafana(c.Request.Context(), ns, email)

	token, err := a.sign(email, ns)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sign token"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"token": token, "namespace": ns})
}

// login verifies credentials and issues a JWT carrying the user's namespace.
func (a *auth) login(c *gin.Context) {
	var in credentials
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))

	var hash, ns string
	err := a.pool.QueryRow(c.Request.Context(),
		`SELECT password_hash, namespace FROM users WHERE email = $1`, email).Scan(&hash, &ns)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup user: " + err.Error()})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	// Re-assert the tenant namespace so an account whose namespace creation
	// failed at registration (or was deleted out-of-band) heals on next login.
	// Best-effort: a transient API error shouldn't block the login itself —
	// shop operations will surface it if the namespace is really gone.
	if err := a.ensureNamespace(c.Request.Context(), ns); err != nil {
		log.Printf("ensure namespace %s on login: %v", ns, err)
	}

	token, err := a.sign(email, ns)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sign token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "namespace": ns})
}

func (a *auth) sign(email, ns string) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		Namespace: ns,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   email,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	return t.SignedString(a.secret)
}

// middleware verifies the Bearer token and stores the caller's namespace in the
// gin context; the Shop handlers read it to scope their operations per tenant.
func (a *auth) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		var cl claims
		tok, err := jwt.ParseWithClaims(raw, &cl, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return a.secret, nil
		})
		if err != nil || !tok.Valid || cl.Namespace == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set("namespace", cl.Namespace)
		c.Next()
	}
}

// randomNamespace returns a DNS-1123 tenant namespace like "tenant-9f3a1b2c".
func randomNamespace() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "tenant-" + hex.EncodeToString(b), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" // unique_violation
	}
	return false
}
