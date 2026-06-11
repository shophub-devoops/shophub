package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "github.com/shophub-devoops/shop-operator/api/apps/v1"
	notifyv1 "github.com/shophub-devoops/shop-operator/api/notify/v1"
	paymentsv1 "github.com/shophub-devoops/shop-operator/api/payments/v1"
)

// This file wires the operator's other two CRDs into the ShopHub flow:
//   - Wallet: "generate a wallet for me" — a Wallet CR whose keypair the
//     operator creates on-chain-side; ShopHub returns the resulting address.
//   - DiscordChannel: opt-in per-shop notification channel — a DiscordChannel
//     CR (channel + webhook on the platform's Discord guild) that the Shop's
//     AlertmanagerConfig routes alerts to.

// walletWaitTimeout bounds how long createWallet waits for the operator to
// publish the generated address before telling the client to retry.
const walletWaitTimeout = 10 * time.Second

// discordConfig is the platform-level Discord setup (one bot, one guild),
// injected by the chart. Empty guildID disables the feature.
type discordConfig struct {
	guildID         string
	botSecretName   string
	botSecretNS     string
}

func (d discordConfig) enabled() bool {
	return d.guildID != "" && d.botSecretName != ""
}

// createWalletRequest names the Wallet CR; a random suffix is used when empty.
type createWalletRequest struct {
	Name string `json:"name"`
}

// createWallet creates a Wallet CR in the caller's tenant namespace and waits
// (bounded) for the operator to generate the keypair and publish the address.
// The user can then point a Shop at the returned address.
func (h *handlers) createWallet(c *gin.Context) {
	ns := nsFromCtx(c)

	var req createWalletRequest
	// Body is optional — ignore bind errors for an empty body.
	_ = c.ShouldBindJSON(&req)
	name := req.Name
	if name == "" {
		suffix := make([]byte, 3)
		if _, err := rand.Read(suffix); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "generate name"})
			return
		}
		name = "wallet-" + hex.EncodeToString(suffix)
	}

	wallet := &paymentsv1.Wallet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       paymentsv1.WalletSpec{Network: paymentsv1.NetworkSepolia},
	}
	if err := h.kube.Create(c.Request.Context(), wallet); err != nil && !apierrors.IsAlreadyExists(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create wallet: " + err.Error()})
		return
	}

	address, err := h.waitForWalletAddress(c.Request.Context(), ns, name)
	if err != nil {
		c.JSON(http.StatusAccepted, gin.H{
			"name":  name,
			"error": "wallet is being provisioned — retry in a few seconds",
		})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"name": name, "address": address})
}

// waitForWalletAddress polls the Wallet CR until the operator publishes
// status.address or the bounded wait expires.
func (h *handlers) waitForWalletAddress(ctx context.Context, ns, name string) (string, error) {
	deadline := time.NewTimer(walletWaitTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	for {
		var w paymentsv1.Wallet
		if err := h.kube.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &w); err == nil && w.Status.Address != "" {
			return w.Status.Address, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline.C:
			return "", context.DeadlineExceeded
		case <-tick.C:
		}
	}
}

// ensureDiscordChannel creates a DiscordChannel CR for the shop (channel +
// webhook on the platform guild, reconciled by the operator) and returns the
// webhook Secret name the Shop CR should reference. The CR is owned by the
// Shop so deleting the shop tears the Discord channel down too (the
// DiscordChannel controller's finalizer deletes the channel on the guild).
func (h *handlers) ensureDiscordChannel(ctx context.Context, shop *appsv1.Shop) (string, error) {
	yes := true
	ch := &notifyv1.DiscordChannel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shop.Name,
			Namespace: shop.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         appsv1.GroupVersion.String(),
				Kind:               "Shop",
				Name:               shop.Name,
				UID:                shop.UID,
				BlockOwnerDeletion: &yes,
			}},
		},
		Spec: notifyv1.DiscordChannelSpec{
			GuildID: h.discord.guildID,
			Name:    shop.Name,
			BotTokenRef: corev1.SecretReference{
				Name:      h.discord.botSecretName,
				Namespace: h.discord.botSecretNS,
			},
		},
	}
	if err := h.kube.Create(ctx, ch); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", err
	}
	// The DiscordChannel controller writes the webhook URL into <name>-webhook;
	// the Shop controller watches that Secret and wires the AlertmanagerConfig.
	return shop.Name + "-webhook", nil
}
