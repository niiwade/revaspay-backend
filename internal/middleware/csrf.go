package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/revaspay/backend/internal/utils"
)

// CSRFConfig contains configuration for CSRF protection
type CSRFConfig struct {
	Secret        string
	CookieName    string
	HeaderName    string
	CookiePath    string
	CookieDomain  string
	CookieSecure  bool
	CookieHTTPOnly bool
	CookieSameSite http.SameSite
	CookieMaxAge  int
	TokenLength   int
	ErrorFunc     func(c *gin.Context)
	ExcludePaths  []string
}

// DefaultCSRFConfig returns the default CSRF configuration
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		Secret:        "", // Must be set by the application
		CookieName:    "csrf_token",
		HeaderName:    "X-CSRF-Token",
		CookiePath:    "/",
		CookieDomain:  "",
		CookieSecure:  true,
		CookieHTTPOnly: true,
		CookieSameSite: http.SameSiteStrictMode,
		CookieMaxAge:  86400, // 24 hours
		TokenLength:   32,
		ErrorFunc:     defaultCSRFErrorFunc,
		ExcludePaths:  []string{},
	}
}

// defaultCSRFErrorFunc is the default error handler for CSRF errors
func defaultCSRFErrorFunc(c *gin.Context) {
	c.JSON(http.StatusForbidden, gin.H{
		"error": "CSRF token validation failed",
	})
	c.Abort()
}

// CSRFMiddleware provides CSRF protection
func CSRFMiddleware(config CSRFConfig) gin.HandlerFunc {
	if config.Secret == "" {
		panic("CSRF middleware requires a secret")
	}

	return func(c *gin.Context) {
		// Skip CSRF check for excluded paths
		for _, path := range config.ExcludePaths {
			if c.Request.URL.Path == path {
				c.Next()
				return
			}
		}

		// Skip CSRF check for safe methods (GET, HEAD, OPTIONS, TRACE)
		if c.Request.Method == "GET" || 
		   c.Request.Method == "HEAD" || 
		   c.Request.Method == "OPTIONS" || 
		   c.Request.Method == "TRACE" {
			// For GET requests, ensure a CSRF token exists
			ensureCSRFToken(c, config)
			c.Next()
			return
		}

		// For unsafe methods, validate the CSRF token
		token := c.GetHeader(config.HeaderName)
		cookie, err := c.Cookie(config.CookieName)

		// If either token is missing, reject the request
		if token == "" || err != nil {
			config.ErrorFunc(c)
			return
		}

		// Validate the token
		if !validateCSRFToken(token, cookie, config.Secret) {
			config.ErrorFunc(c)
			return
		}

		// Token is valid, proceed
		c.Next()
	}
}

// ensureCSRFToken ensures a CSRF token exists in the cookie
func ensureCSRFToken(c *gin.Context, config CSRFConfig) {
	// Check if the token already exists
	_, err := c.Cookie(config.CookieName)
	if err == nil {
		// Token exists, no need to create a new one
		return
	}

	// Generate a new token
	token, err := generateCSRFToken(config.TokenLength)
	if err != nil {
		// If we can't generate a token, fail open (better than blocking all requests)
		return
	}

	// Create a signed token
	signedToken := utils.SignHMAC(token, config.Secret)

	// Set the cookie
	c.SetSameSite(config.CookieSameSite)
	c.SetCookie(
		config.CookieName,
		signedToken,
		config.CookieMaxAge,
		config.CookiePath,
		config.CookieDomain,
		config.CookieSecure,
		config.CookieHTTPOnly,
	)
}

// generateCSRFToken generates a random token
func generateCSRFToken(length int) (string, error) {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// validateCSRFToken validates a CSRF token
func validateCSRFToken(token, cookie, secret string) bool {
	// Constant-time comparison to prevent timing attacks
	return utils.VerifyHMAC(token, cookie, secret)
}

// GetCSRFToken returns the current CSRF token
func GetCSRFToken(c *gin.Context, config CSRFConfig) (string, error) {
	token, err := c.Cookie(config.CookieName)
	if err != nil {
		return "", errors.New("CSRF token not found")
	}
	return token, nil
}

// RegenerateCSRFToken regenerates the CSRF token
func RegenerateCSRFToken(c *gin.Context, config CSRFConfig) (string, error) {
	// Delete the existing cookie
	c.SetCookie(
		config.CookieName,
		"",
		-1,
		config.CookiePath,
		config.CookieDomain,
		config.CookieSecure,
		config.CookieHTTPOnly,
	)

	// Generate a new token
	token, err := generateCSRFToken(config.TokenLength)
	if err != nil {
		return "", err
	}

	// Create a signed token
	signedToken := utils.SignHMAC(token, config.Secret)

	// Set the cookie
	c.SetSameSite(config.CookieSameSite)
	c.SetCookie(
		config.CookieName,
		signedToken,
		config.CookieMaxAge,
		config.CookiePath,
		config.CookieDomain,
		config.CookieSecure,
		config.CookieHTTPOnly,
	)

	return signedToken, nil
}
