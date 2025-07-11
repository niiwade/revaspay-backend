package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

// SecureHeadersConfig contains configuration for secure headers
type SecureHeadersConfig struct {
	// HSTS settings
	UseHSTS          bool
	HSTSMaxAge       time.Duration
	HSTSIncludeSubdomains bool
	HSTSPreload      bool

	// CSP settings
	UseCSP           bool
	CSPDirectives    map[string]string

	// Other security headers
	UseXFrameOptions bool
	XFrameOptions    string
	UseXSSProtection bool
	UseNoSniff       bool
	UseCORS          bool
	UseReferrerPolicy bool
	ReferrerPolicy   string
	UsePermissionsPolicy bool
	PermissionsPolicy string
}

// DefaultSecureHeadersConfig returns the default secure headers configuration
func DefaultSecureHeadersConfig() SecureHeadersConfig {
	return SecureHeadersConfig{
		UseHSTS:          true,
		HSTSMaxAge:       365 * 24 * time.Hour, // 1 year
		HSTSIncludeSubdomains: true,
		HSTSPreload:      true,

		UseCSP:           true,
		CSPDirectives: map[string]string{
			"default-src": "'self'",
			"script-src":  "'self' 'unsafe-inline' https://cdn.jsdelivr.net https://js.stripe.com",
			"style-src":   "'self' 'unsafe-inline' https://fonts.googleapis.com https://cdn.jsdelivr.net",
			"img-src":     "'self' data: https://res.cloudinary.com",
			"font-src":    "'self' https://fonts.gstatic.com",
			"connect-src": "'self' https://api.revaspay.com https://api.stripe.com",
			"frame-src":   "'self' https://js.stripe.com",
			"object-src":  "'none'",
			"base-uri":    "'self'",
			"form-action": "'self'",
		},

		UseXFrameOptions: true,
		XFrameOptions:    "DENY",
		UseXSSProtection: true,
		UseNoSniff:       true,
		UseCORS:          true,
		UseReferrerPolicy: true,
		ReferrerPolicy:   "strict-origin-when-cross-origin",
		UsePermissionsPolicy: true,
		PermissionsPolicy: "camera=(), microphone=(), geolocation=(), interest-cohort=()",
	}
}

// SecureHeadersMiddleware adds security headers to responses
func SecureHeadersMiddleware(config SecureHeadersConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// HTTP Strict Transport Security (HSTS)
		if config.UseHSTS {
			maxAge := int64(config.HSTSMaxAge.Seconds())
			value := "max-age=" + string(rune(maxAge))
			if config.HSTSIncludeSubdomains {
				value += "; includeSubDomains"
			}
			if config.HSTSPreload {
				value += "; preload"
			}
			c.Header("Strict-Transport-Security", value)
		}

		// Content Security Policy (CSP)
		if config.UseCSP {
			var cspValue string
			for directive, value := range config.CSPDirectives {
				if cspValue != "" {
					cspValue += "; "
				}
				cspValue += directive + " " + value
			}
			c.Header("Content-Security-Policy", cspValue)
		}

		// X-Frame-Options to prevent clickjacking
		if config.UseXFrameOptions {
			c.Header("X-Frame-Options", config.XFrameOptions)
		}

		// X-XSS-Protection to enable browser's XSS filtering
		if config.UseXSSProtection {
			c.Header("X-XSS-Protection", "1; mode=block")
		}

		// X-Content-Type-Options to prevent MIME type sniffing
		if config.UseNoSniff {
			c.Header("X-Content-Type-Options", "nosniff")
		}

		// Referrer-Policy to control how much referrer information is included
		if config.UseReferrerPolicy {
			c.Header("Referrer-Policy", config.ReferrerPolicy)
		}

		// Permissions-Policy to control browser features
		if config.UsePermissionsPolicy {
			c.Header("Permissions-Policy", config.PermissionsPolicy)
		}

		// Cache control for sensitive pages
		if c.Request.URL.Path == "/api/login" || 
		   c.Request.URL.Path == "/api/signup" || 
		   c.Request.URL.Path == "/api/reset-password" {
			c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		}

		c.Next()
	}
}

// CORSMiddleware handles Cross-Origin Resource Sharing
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		
		// Check if the origin is in the allowed list
		allowed := false
		for _, allowedOrigin := range allowedOrigins {
			if origin == allowedOrigin {
				allowed = true
				break
			}
		}
		
		// If origin is allowed or we're allowing all origins
		if allowed || len(allowedOrigins) == 0 {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}
		
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		
		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		
		c.Next()
	}
}
