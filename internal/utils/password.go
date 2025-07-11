package utils

import (
	"errors"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// PasswordHashCost defines the cost for bcrypt password hashing
// Higher values are more secure but slower
const PasswordHashCost = 12 // Increased from default 10 for better security

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), PasswordHashCost)
	return string(bytes), err
}

// CheckPasswordHash compares a password with a hash
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// PasswordPolicy defines the requirements for password strength
type PasswordPolicy struct {
	MinLength         int
	RequireUppercase  bool
	RequireLowercase  bool
	RequireNumbers    bool
	RequireSpecial    bool
	DisallowCommon    bool
	MaxRepeatedChars  int
	DisallowUsername  bool
	DisallowPersonal  bool
	CommonPasswords   map[string]bool // For checking against common passwords
	PersonalKeywords  []string        // User-related words to avoid (name, etc.)
}

// DefaultPasswordPolicy returns the default password policy
func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{
		MinLength:        12,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireNumbers:   true,
		RequireSpecial:   true,
		DisallowCommon:   true,
		MaxRepeatedChars: 3,
		DisallowUsername: true,
		DisallowPersonal: true,
		CommonPasswords:  loadCommonPasswords(),
	}
}

// ValidatePassword checks if a password meets the policy requirements
func (p PasswordPolicy) ValidatePassword(password, username string, personalInfo []string) error {
	// Check length
	if len(password) < p.MinLength {
		return errors.New("password must be at least " + string(rune(p.MinLength)) + " characters long")
	}

	// Check character requirements
	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if p.RequireUppercase && !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if p.RequireLowercase && !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if p.RequireNumbers && !hasNumber {
		return errors.New("password must contain at least one number")
	}
	if p.RequireSpecial && !hasSpecial {
		return errors.New("password must contain at least one special character")
	}

	// Check for repeated characters
	if p.MaxRepeatedChars > 0 {
		for i := 0; i <= len(password)-p.MaxRepeatedChars; i++ {
			if allSameChars(password[i : i+p.MaxRepeatedChars]) {
				return errors.New("password contains too many repeated characters in sequence")
			}
		}
	}

	// Check against common passwords
	if p.DisallowCommon && p.CommonPasswords != nil {
		if p.CommonPasswords[strings.ToLower(password)] {
			return errors.New("password is too common and easily guessable")
		}
	}

	// Check if password contains username
	if p.DisallowUsername && strings.Contains(strings.ToLower(password), strings.ToLower(username)) {
		return errors.New("password should not contain your username")
	}

	// Check if password contains personal information
	if p.DisallowPersonal {
		for _, info := range personalInfo {
			if info != "" && strings.Contains(strings.ToLower(password), strings.ToLower(info)) {
				return errors.New("password should not contain personal information")
			}
		}
	}

	return nil
}

// Helper function to check if all characters in a string are the same
func allSameChars(s string) bool {
	if len(s) <= 1 {
		return true
	}
	first := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] != first {
			return false
		}
	}
	return true
}

// loadCommonPasswords loads a map of common passwords to check against
// In a real implementation, this would load from a file or database
func loadCommonPasswords() map[string]bool {
	// This is a small sample - in production, use a much larger list
	commonPwds := []string{
		"password", "123456", "qwerty", "admin", "welcome",
		"password123", "abc123", "letmein", "monkey", "1234567890",
		"trustno1", "sunshine", "master", "123123", "welcome1",
		"password1", "admin123", "qwerty123", "football", "iloveyou",
	}

	result := make(map[string]bool)
	for _, pwd := range commonPwds {
		result[pwd] = true
	}
	return result
}

// GenerateSecurePassword generates a cryptographically secure random password
// that meets the password policy requirements
func GenerateSecurePassword(length int) (string, error) {
	if length < 12 {
		length = 12 // Minimum secure length
	}
	
	// Implementation would use crypto/rand to generate a secure random password
	// This is a placeholder for the actual implementation
	return "SecureRandomPassword", nil
}

// IsPasswordPwned checks if a password has been exposed in data breaches
// This would typically use a service like the "Have I Been Pwned" API
func IsPasswordPwned(password string) (bool, error) {
	// Implementation would check against a password breach database
	// This is a placeholder for the actual implementation
	return false, nil
}
