package utils

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"image/png"
	"net/url"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// MFAConfig holds configuration for multi-factor authentication
type MFAConfig struct {
	Issuer         string
	Period         uint
	Digits         otp.Digits
	Algorithm      otp.Algorithm
	SecretSize     uint
	BackupCodeCount int
}

// DefaultMFAConfig returns the default MFA configuration
func DefaultMFAConfig() MFAConfig {
	return MFAConfig{
		Issuer:         "RevasPay",
		Period:         30,
		Digits:         otp.DigitsSix,
		Algorithm:      otp.AlgorithmSHA1,
		SecretSize:     20,
		BackupCodeCount: 10,
	}
}

// MFAKey represents a TOTP key for multi-factor authentication
type MFAKey struct {
	Secret     string
	URL        string
	QRCode     []byte
	BackupCodes []string
}

// GenerateTOTPKey generates a new TOTP key for MFA
func GenerateTOTPKey(config MFAConfig, accountName string) (*MFAKey, error) {
	// Generate TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      config.Issuer,
		AccountName: accountName,
		Period:      config.Period,
		Digits:      config.Digits,
		Algorithm:   config.Algorithm,
		SecretSize:  config.SecretSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	// Generate backup codes
	backupCodes, err := GenerateBackupCodes(config.BackupCodeCount)
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	// Get QR code image
	qrCode, err := key.Image(200, 200)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	// Convert QR code to PNG bytes
	var qrBytes []byte
	buf := new(bytes.Buffer)
	err = png.Encode(buf, qrCode)
	if err != nil {
		return nil, fmt.Errorf("failed to encode QR code: %w", err)
	}
	qrBytes = buf.Bytes()

	return &MFAKey{
		Secret:     key.Secret(),
		URL:        key.URL(),
		QRCode:     qrBytes,
		BackupCodes: backupCodes,
	}, nil
}

// ValidateTOTPCode validates a TOTP code
func ValidateTOTPCode(secret, code string, config MFAConfig) bool {
	// Remove spaces from the code
	code = strings.ReplaceAll(code, " ", "")

	// Validate the TOTP code
	valid, err := totp.ValidateCustom(
		code,
		secret,
		time.Now().UTC(),
		totp.ValidateOpts{
			Period:    config.Period,
			Digits:    config.Digits,
			Algorithm: config.Algorithm,
		},
	)
	if err != nil {
		return false
	}

	return valid
}

// GenerateBackupCodes generates random backup codes for MFA recovery
func GenerateBackupCodes(count int) ([]string, error) {
	codes := make([]string, count)
	
	for i := 0; i < count; i++ {
		// Generate 10 random bytes
		bytes := make([]byte, 5)
		_, err := rand.Read(bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to generate random bytes: %w", err)
		}
		
		// Convert to base32 and take first 10 characters
		code := strings.ToUpper(base32.StdEncoding.EncodeToString(bytes))
		code = code[:10]
		
		// Format as XXXXX-XXXXX
		codes[i] = fmt.Sprintf("%s-%s", code[:5], code[5:10])
	}
	
	return codes, nil
}

// ParseTOTPURL parses a TOTP URL into its components
func ParseTOTPURL(totpURL string) (issuer, accountName, secret string, err error) {
	u, err := url.Parse(totpURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid TOTP URL: %w", err)
	}
	
	if u.Scheme != "otpauth" || u.Host != "totp" {
		return "", "", "", fmt.Errorf("invalid TOTP URL scheme or host")
	}
	
	// Parse path to get account name
	path := strings.TrimPrefix(u.Path, "/")
	parts := strings.SplitN(path, ":", 2)
	
	if len(parts) > 1 {
		issuer = parts[0]
		accountName = parts[1]
	} else {
		accountName = parts[0]
	}
	
	// Parse query parameters
	query := u.Query()
	secret = query.Get("secret")
	
	if issuerParam := query.Get("issuer"); issuerParam != "" {
		issuer = issuerParam
	}
	
	if secret == "" {
		return "", "", "", fmt.Errorf("missing secret in TOTP URL")
	}
	
	return issuer, accountName, secret, nil
}

// GenerateRecoveryToken generates a recovery token for MFA reset
func GenerateRecoveryToken() (string, error) {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	
	// Convert to URL-safe base64
	token := base64.URLEncoding.EncodeToString(bytes)
	
	return token, nil
}
