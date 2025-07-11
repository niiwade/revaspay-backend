package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter implements rate limiting for API endpoints
type RateLimiter struct {
	ipLimiters     map[string]*rate.Limiter
	authLimiters   map[string]*rate.Limiter
	ipMutex        sync.RWMutex
	authMutex      sync.RWMutex
	ipLimiterRate  rate.Limit
	authLimiterRate rate.Limit
	ipBurst        int
	authBurst      int
	cleanupTicker  *time.Ticker
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(ipRequestsPerSecond, authRequestsPerMinute float64, ipBurst, authBurst int) *RateLimiter {
	limiter := &RateLimiter{
		ipLimiters:     make(map[string]*rate.Limiter),
		authLimiters:   make(map[string]*rate.Limiter),
		ipLimiterRate:  rate.Limit(ipRequestsPerSecond),
		authLimiterRate: rate.Limit(authRequestsPerMinute / 60), // Convert to per-second rate
		ipBurst:        ipBurst,
		authBurst:      authBurst,
		cleanupTicker:  time.NewTicker(5 * time.Minute),
	}

	// Start cleanup goroutine
	go limiter.cleanup()
	
	return limiter
}

// cleanup periodically removes old limiters to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	for range rl.cleanupTicker.C {
		rl.ipMutex.Lock()
		rl.ipLimiters = make(map[string]*rate.Limiter)
		rl.ipMutex.Unlock()

		rl.authMutex.Lock()
		rl.authLimiters = make(map[string]*rate.Limiter)
		rl.authMutex.Unlock()
	}
}

// Stop stops the rate limiter cleanup
func (rl *RateLimiter) Stop() {
	rl.cleanupTicker.Stop()
}

// getIPLimiter returns the rate limiter for an IP
func (rl *RateLimiter) getIPLimiter(ip string) *rate.Limiter {
	rl.ipMutex.RLock()
	limiter, exists := rl.ipLimiters[ip]
	rl.ipMutex.RUnlock()

	if !exists {
		rl.ipMutex.Lock()
		limiter = rate.NewLimiter(rl.ipLimiterRate, rl.ipBurst)
		rl.ipLimiters[ip] = limiter
		rl.ipMutex.Unlock()
	}

	return limiter
}

// getAuthLimiter returns the rate limiter for authentication attempts
func (rl *RateLimiter) getAuthLimiter(key string) *rate.Limiter {
	rl.authMutex.RLock()
	limiter, exists := rl.authLimiters[key]
	rl.authMutex.RUnlock()

	if !exists {
		rl.authMutex.Lock()
		limiter = rate.NewLimiter(rl.authLimiterRate, rl.authBurst)
		rl.authLimiters[key] = limiter
		rl.authMutex.Unlock()
	}

	return limiter
}

// IPRateLimiterMiddleware limits requests based on IP address
func (rl *RateLimiter) IPRateLimiterMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := rl.getIPLimiter(ip)
		
		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// AuthRateLimiterMiddleware limits authentication attempts based on IP and username/email
func (rl *RateLimiter) AuthRateLimiterMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		
		// For auth endpoints, we want to rate limit by IP first
		ipLimiter := rl.getIPLimiter(ip)
		if !ipLimiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			c.Abort()
			return
		}
		
		// For login/signup endpoints, also rate limit by the provided email/username
		// This prevents someone from trying many passwords for a single account
		if c.Request.Method == "POST" {
			var requestBody struct {
				Email    string `json:"email"`
				Username string `json:"username"`
			}
			
			if err := c.ShouldBindJSON(&requestBody); err == nil {
				identifier := requestBody.Email
				if identifier == "" {
					identifier = requestBody.Username
				}
				
				if identifier != "" {
					// Create a key that combines IP and identifier
					key := ip + ":" + identifier
					authLimiter := rl.getAuthLimiter(key)
					
					if !authLimiter.Allow() {
						c.JSON(http.StatusTooManyRequests, gin.H{
							"error": "too many authentication attempts, please try again later",
						})
						c.Abort()
						return
					}
				}
			}
			
			// Reset the request body for the next middleware to read
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20) // 1MB limit
		}
		
		c.Next()
	}
}
