package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "github.com/shophub-devoops/shop-operator/api/apps/v1"
)

// handlers carries the dependencies shared by all HTTP endpoints. The tenant
// namespace is not a field — it comes from the authenticated caller's JWT
// (set by the auth middleware) so each user only touches their own shops.
type handlers struct {
	kube client.Client
}

// nsFromCtx returns the caller's tenant namespace, set by the auth middleware.
func nsFromCtx(c *gin.Context) string {
	v, _ := c.Get("namespace")
	ns, _ := v.(string)
	return ns
}

// createShopRequest is the payload for POST /api/shops.
type createShopRequest struct {
	Name          string `json:"name" binding:"required"`
	Title         string `json:"title" binding:"required"`
	Availability  string `json:"availability" binding:"required,oneof=standard high"`
	Database      string `json:"database" binding:"required,oneof=postgres mongodb"`
	WalletAddress string `json:"walletAddress" binding:"required"`
}

// updateShopRequest is the payload for PUT /api/shops/:name. Name is bound
// to the URL param, not the body.
type updateShopRequest struct {
	Title         *string `json:"title,omitempty"`
	Availability  *string `json:"availability,omitempty" binding:"omitempty,oneof=standard high"`
	WalletAddress *string `json:"walletAddress,omitempty"`
}

// shopResponse is the public view of a Shop CR. Database kind is fixed at
// creation; we don't expose changing it (would require destroying data).
type shopResponse struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Title         string `json:"title"`
	Availability  string `json:"availability"`
	Database      string `json:"database"`
	WalletAddress string `json:"walletAddress"`
	URL           string `json:"url,omitempty"`
	ReadyReplicas int32  `json:"readyReplicas"`
}

func toResponse(s *appsv1.Shop) shopResponse {
	return shopResponse{
		Name:          s.Name,
		Namespace:     s.Namespace,
		Title:         s.Spec.Title,
		Availability:  string(s.Spec.Availability),
		Database:      string(s.Spec.Database),
		WalletAddress: s.Spec.WalletAddress,
		URL:           s.Status.URL,
		ReadyReplicas: s.Status.ReadyReplicas,
	}
}

func (h *handlers) listShops(c *gin.Context) {
	var list appsv1.ShopList
	if err := h.kube.List(c.Request.Context(), &list, client.InNamespace(nsFromCtx(c))); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list shops: " + err.Error()})
		return
	}
	out := make([]shopResponse, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, toResponse(&list.Items[i]))
	}
	c.JSON(http.StatusOK, out)
}

func (h *handlers) getShop(c *gin.Context) {
	var shop appsv1.Shop
	err := h.kube.Get(c.Request.Context(), client.ObjectKey{Namespace: nsFromCtx(c), Name: c.Param("name")}, &shop)
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "shop not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get shop: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, toResponse(&shop))
}

func (h *handlers) createShop(c *gin.Context) {
	var req createShopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	shop := &appsv1.Shop{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: nsFromCtx(c),
		},
		Spec: appsv1.ShopSpec{
			Title:         req.Title,
			Availability:  appsv1.Availability(req.Availability),
			Database:      appsv1.DatabaseKind(req.Database),
			WalletAddress: req.WalletAddress,
		},
	}
	if err := h.kube.Create(c.Request.Context(), shop); err != nil {
		if apierrors.IsAlreadyExists(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "shop already exists"})
			return
		}
		if apierrors.IsInvalid(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create shop: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, toResponse(shop))
}

func (h *handlers) updateShop(c *gin.Context) {
	var req updateShopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var shop appsv1.Shop
	err := h.kube.Get(c.Request.Context(), client.ObjectKey{Namespace: nsFromCtx(c), Name: c.Param("name")}, &shop)
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "shop not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get shop: " + err.Error()})
		return
	}

	if req.Title != nil {
		shop.Spec.Title = *req.Title
	}
	if req.Availability != nil {
		shop.Spec.Availability = appsv1.Availability(*req.Availability)
	}
	if req.WalletAddress != nil {
		shop.Spec.WalletAddress = *req.WalletAddress
	}

	if err := h.kube.Update(c.Request.Context(), &shop); err != nil {
		if apierrors.IsConflict(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict — refetch and retry"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update shop: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, toResponse(&shop))
}

func (h *handlers) deleteShop(c *gin.Context) {
	shop := &appsv1.Shop{
		ObjectMeta: metav1.ObjectMeta{Name: c.Param("name"), Namespace: nsFromCtx(c)},
	}
	err := h.kube.Delete(c.Request.Context(), shop)
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "shop not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete shop: " + err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
