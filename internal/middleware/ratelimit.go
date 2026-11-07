package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type clientVisitor struct {
	lastSeen time.Time
	count    int
}

type IPRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*clientVisitor
	limit    int
	window   time.Duration
}

func NewIPRateLimiter(limit int, window time.Duration) *IPRateLimiter {
	limiter := &IPRateLimiter{
		visitors: make(map[string]*clientVisitor),
		limit:    limit,
		window:   window,
	}

	// Periodically cleanup expired entries
	go func() {
		for {
			time.Sleep(limiter.window)
			limiter.mu.Lock()
			now := time.Now()
			for ip, v := range limiter.visitors {
				if now.Sub(v.lastSeen) > limiter.window {
					delete(limiter.visitors, ip)
				}
			}
			limiter.mu.Unlock()
		}
	}()

	return limiter
}

func (l *IPRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	v, exists := l.visitors[ip]
	if !exists || now.Sub(v.lastSeen) > l.window {
		l.visitors[ip] = &clientVisitor{lastSeen: now, count: 1}
		return true
	}

	if v.count >= l.limit {
		return false
	}

	v.count++
	return true
}

func RateLimitMiddleware(limiter *IPRateLimiter, message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.Allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "Too Many Requests",
				"message": message,
			})
			return
		}
		c.Next()
	}
}
