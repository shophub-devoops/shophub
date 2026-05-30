package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const tokenTTL = 24 * time.Hour

// auth carries auth dependencies: the user store (Postgres), the kube client
// (to create each user's tenant namespace), and the JWT signing secret.
type auth struct {
	pool   *pgxpool.Pool
	kube   client.Client
	secret []byte
}

// ensureUsersSchema creates the users table on startup. Each user owns one
// tenant namespace where their Shop CRs live.
func ensureUsersSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id            bigserial PRIMARY KEY,
			email         text UNIQUE NOT NULL,
			password_hash text NOT NULL,
			namespace     text UNIQUE NOT NULL,
			created_at    timestamptz NOT NULL DEFAULT now()
		)`)
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

	// Materialize the tenant namespace (idempotent).
	if err := a.kube.Create(c.Request.Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}); err != nil && !apierrors.IsAlreadyExists(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create namespace: " + err.Error()})
		return
	}

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
