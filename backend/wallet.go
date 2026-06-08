package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Web3 wallet sign-in (spec 1.1 optional): the user proves ownership of an
// address by signing a server-issued nonce with MetaMask (EIP-191 personal_sign).
// No password — the wallet address is the identity, mapped to its own tenant
// namespace exactly like an email user.

const nonceTTL = 5 * time.Minute

// Login nonces are stateless: instead of storing them server-side, the nonce
// endpoint hands the client an HMAC-signed token binding (address, nonce,
// expiry). Step 2 returns the token, and any replica can verify it with the
// shared signing secret — so wallet sign-in works across multiple ShopHub
// replicas without a shared nonce store.

// issueNonce returns a fresh random nonce plus a token that authenticates it:
// base64url(address|nonce|expiry) "." hex(HMAC-SHA256(secret, payload)).
func (a *auth) issueNonce(addr string) (nonce, tokenStr string, err error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	nonce = hex.EncodeToString(b)
	payload := fmt.Sprintf("%s|%s|%d", addr, nonce, time.Now().Add(nonceTTL).Unix())
	tokenStr = base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + a.nonceMAC(payload)
	return nonce, tokenStr, nil
}

// verifyNonce validates the token's HMAC, bound address and expiry, returning
// the nonce it carries. The MAC is compared in constant time.
func (a *auth) verifyNonce(addr, tokenStr string) (nonce string, ok bool) {
	enc, mac, found := strings.Cut(tokenStr, ".")
	if !found {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil || !hmac.Equal([]byte(mac), []byte(a.nonceMAC(string(payload)))) {
		return "", false
	}
	parts := strings.Split(string(payload), "|")
	if len(parts) != 3 || !strings.EqualFold(parts[0], addr) {
		return "", false
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	return parts[1], true
}

// nonceMAC is the HMAC-SHA256 of payload under the JWT signing secret, hex-encoded.
func (a *auth) nonceMAC(payload string) string {
	m := hmac.New(sha256.New, a.secret)
	m.Write([]byte(payload))
	return hex.EncodeToString(m.Sum(nil))
}

// signMessage is the exact text the wallet signs; must match on both sides.
func signMessage(nonce string) string {
	return "Sign in to ShopHub.\n\nNonce: " + nonce
}

type nonceRequest struct {
	Address string `json:"address" binding:"required"`
}

// nonce issues a login nonce for an address (step 1 of wallet sign-in).
func (a *auth) nonce(c *gin.Context) {
	var in nonceRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	addr := strings.ToLower(strings.TrimSpace(in.Address))
	n, tok, err := a.issueNonce(addr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate nonce"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nonce": n, "message": signMessage(n), "token": tok})
}

type walletRequest struct {
	Address   string `json:"address" binding:"required"`
	Signature string `json:"signature" binding:"required"`
	// Token is the HMAC-signed nonce token from the /nonce response (step 1).
	Token string `json:"token" binding:"required"`
}

// walletLogin verifies the signed nonce, then logs the wallet in (step 2):
// upserts a user keyed by the address and issues a JWT carrying its namespace.
func (a *auth) walletLogin(c *gin.Context) {
	var in walletRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	addr := strings.ToLower(strings.TrimSpace(in.Address))

	nonce, ok := a.verifyNonce(addr, in.Token)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "nonce expired or invalid — request a new one"})
		return
	}
	if !verifySignature(addr, signMessage(nonce), in.Signature) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "signature does not match address"})
		return
	}

	ns, err := a.ensureWalletUser(c.Request.Context(), addr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "provision wallet user: " + err.Error()})
		return
	}

	token, err := a.sign(addr, ns)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sign token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "namespace": ns})
}

// ensureWalletUser finds (or creates) the user row for a wallet address and
// returns its tenant namespace. The address is stored in the email column
// (lowercased) so wallet and email users share one users table; password_hash
// is empty since wallet users never password-login. Materializes the namespace.
func (a *auth) ensureWalletUser(ctx context.Context, addr string) (string, error) {
	var ns string
	err := a.pool.QueryRow(ctx, `SELECT namespace FROM users WHERE email = $1`, addr).Scan(&ns)
	if err == nil {
		return ns, a.ensureNamespace(ctx, ns)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	ns, err = randomNamespace()
	if err != nil {
		return "", err
	}
	_, err = a.pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, namespace) VALUES ($1, '', $2)`, addr, ns)
	if isUniqueViolation(err) {
		// Concurrent first login for the same wallet — re-read the winner's row.
		if e := a.pool.QueryRow(ctx, `SELECT namespace FROM users WHERE email = $1`, addr).Scan(&ns); e != nil {
			return "", e
		}
		return ns, a.ensureNamespace(ctx, ns)
	}
	if err != nil {
		return "", err
	}
	return ns, a.ensureNamespace(ctx, ns)
}

// ensureNamespace materializes the tenant namespace (idempotent), mirroring
// what register does for email users.
func (a *auth) ensureNamespace(ctx context.Context, ns string) error {
	err := a.kube.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// verifySignature reports whether sigHex is a valid EIP-191 personal_sign of
// message by addr (the MetaMask signing scheme).
func verifySignature(addr, message, sigHex string) bool {
	sig, err := hexutil.Decode(sigHex)
	if err != nil || len(sig) != 65 {
		return false
	}
	// MetaMask returns V as 27/28; go-ethereum's recovery wants 0/1.
	if sig[64] == 27 || sig[64] == 28 {
		sig[64] -= 27
	}
	pub, err := crypto.SigToPub(accounts.TextHash([]byte(message)), sig)
	if err != nil {
		return false
	}
	return strings.EqualFold(crypto.PubkeyToAddress(*pub).Hex(), addr)
}
