package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// SignHMAC creates an HMAC signature for a message using the provided secret
func SignHMAC(message, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// VerifyHMAC verifies an HMAC signature against a message using the provided secret
// Uses constant-time comparison to prevent timing attacks
func VerifyHMAC(message, signature, secret string) bool {
	expectedMAC := SignHMAC(message, secret)
	
	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expectedMAC)) == 1
}
