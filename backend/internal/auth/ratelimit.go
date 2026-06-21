package auth

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimit guards the unauthenticated auth endpoints. Registration is the
// expensive one — every call inserts a user and creates a tenant namespace in
// the cluster — so unbounded anonymous calls could exhaust cluster resources.
// Per-client-IP token bucket: `burst` immediate requests, then one per `every`.
// In-memory and per-replica, which is fine for this scale — the point is
// stopping runaway loops, not precise global quotas.
type RateLimit struct {
	mu      sync.Mutex
	clients map[string]*rate.Limiter
	rate    rate.Limit
	burst   int
}

// maxTrackedClients caps the limiter map so a spray of spoofed IPs can't grow
// it without bound; on overflow the map resets (brief amnesty, bounded memory).
const maxTrackedClients = 10000

func NewRateLimit(perSecond float64, burst int) *RateLimit {
	return &RateLimit{
		clients: make(map[string]*rate.Limiter),
		rate:    rate.Limit(perSecond),
		burst:   burst,
	}
}

func (rl *RateLimit) limiterFor(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if len(rl.clients) > maxTrackedClients {
		rl.clients = make(map[string]*rate.Limiter)
	}
	l, ok := rl.clients[ip]
	if !ok {
		l = rate.NewLimiter(rl.rate, rl.burst)
		rl.clients[ip] = l
	}
	return l
}

func (rl *RateLimit) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.limiterFor(c.ClientIP()).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests — slow down"})
			return
		}
		c.Next()
	}
}
