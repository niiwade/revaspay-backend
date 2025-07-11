package utils

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
)

// GenerateRandomString creates a random string of specified length
func GenerateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:length], nil
}

// GenerateReferralCode creates a unique referral code
func GenerateReferralCode(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	
	return string(result)
}

// FormatCurrency formats a float as currency
func FormatCurrency(amount float64, currency string) string {
	switch currency {
	case "USD":
		return fmt.Sprintf("$%.2f", amount)
	case "EUR":
		return fmt.Sprintf("€%.2f", amount)
	case "GBP":
		return fmt.Sprintf("£%.2f", amount)
	default:
		return fmt.Sprintf("%.2f %s", amount, currency)
	}
}

// GenerateTransactionReference creates a unique transaction reference
func GenerateTransactionReference(prefix string) string {
	timestamp := time.Now().Format("20060102150405")
	random, _ := GenerateRandomString(8)
	return strings.ToUpper(fmt.Sprintf("%s_%s_%s", prefix, timestamp, random))
}

// TruncateString truncates a string to the specified length
func TruncateString(str string, length int) string {
	if len(str) <= length {
		return str
	}
	return str[:length-3] + "..."
}

// IsValidEmail checks if an email address is valid
func IsValidEmail(email string) bool {
	// Simple validation - contains @ and at least one dot after @
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	
	domainParts := strings.Split(parts[1], ".")
	return len(domainParts) >= 2 && domainParts[len(domainParts)-1] != ""
}

// CalculateFee calculates the fee for a transaction
func CalculateFee(amount float64, feePercentage float64) float64 {
	return amount * (feePercentage / 100.0)
}

// GenerateSecureToken generates a secure random token of specified length
func GenerateSecureToken(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)[:length]
}

// GenerateUsername creates a username from an email address
func GenerateUsername(email string) string {
	// Extract the part before @ in the email
	parts := strings.Split(email, "@")
	baseName := parts[0]
	
	// Remove special characters
	reg, _ := regexp.Compile("[^a-zA-Z0-9]+")
	baseName = reg.ReplaceAllString(baseName, "")
	
	// Add random suffix
	random, _ := GenerateRandomString(4)
	return strings.ToLower(baseName + random)
}

// GenerateOTPSecret generates a new TOTP secret
func GenerateOTPSecret() string {
	// Generate 20 random bytes
	secretBytes := make([]byte, 20)
	rand.Read(secretBytes)
	
	// Encode as base32 (standard for TOTP)
	return base32.StdEncoding.EncodeToString(secretBytes)
}

// ValidateTOTP validates a TOTP code against a secret
func ValidateTOTP(secret string, code string) bool {
	return totp.Validate(code, secret)
}

// GenerateOTPQRCode generates a URL for a QR code for TOTP setup
func GenerateOTPQRCode(secret string, accountName string, issuer string) string {
	// Format: otpauth://totp/ISSUER:ACCOUNT?secret=SECRET&issuer=ISSUER
	accountName = url.QueryEscape(accountName)
	issuer = url.QueryEscape(issuer)
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s", 
		issuer, accountName, secret, issuer)
}

// GenerateEmailVerificationToken generates a token for email verification
func GenerateEmailVerificationToken() string {
	token, _ := GenerateRandomString(32)
	return token
}

// GeneratePasswordResetToken generates a token for password reset
func GeneratePasswordResetToken() string {
	token, _ := GenerateRandomString(32)
	return token
}
