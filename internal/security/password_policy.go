package security

import (
	"errors"
	"regexp"
	"strings"
)

// PasswordPolicy defines the requirements for a strong password
type PasswordPolicy struct {
	MinLength         int  // Minimum password length
	RequireUppercase  bool // Require at least one uppercase letter
	RequireLowercase  bool // Require at least one lowercase letter
	RequireDigit      bool // Require at least one digit
	RequireSpecial    bool // Require at least one special character
	DisallowCommon    bool // Disallow common passwords
	MaxRepeatedChars  int  // Maximum number of repeated characters
	DisallowUsername  bool // Disallow username in password
	DisallowEmail     bool // Disallow email in password
	DisallowSequences bool // Disallow common sequences like "123456" or "qwerty"
}

// DefaultPasswordPolicy returns a default password policy
func DefaultPasswordPolicy() *PasswordPolicy {
	return &PasswordPolicy{
		MinLength:         10,
		RequireUppercase:  true,
		RequireLowercase:  true,
		RequireDigit:      true,
		RequireSpecial:    true,
		DisallowCommon:    true,
		MaxRepeatedChars:  3,
		DisallowUsername:  true,
		DisallowEmail:     true,
		DisallowSequences: true,
	}
}

// LenientPasswordPolicy returns a more lenient password policy
func LenientPasswordPolicy() *PasswordPolicy {
	return &PasswordPolicy{
		MinLength:         8,
		RequireUppercase:  true,
		RequireLowercase:  true,
		RequireDigit:      true,
		RequireSpecial:    false,
		DisallowCommon:    true,
		MaxRepeatedChars:  4,
		DisallowUsername:  true,
		DisallowEmail:     true,
		DisallowSequences: true,
	}
}

// ValidatePassword validates a password against the policy
func (p *PasswordPolicy) ValidatePassword(password, username, email string) error {
	// Check minimum length
	if len(password) < p.MinLength {
		return errors.New("password is too short")
	}

	// Check for uppercase letters
	if p.RequireUppercase {
		hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
		if !hasUpper {
			return errors.New("password must contain at least one uppercase letter")
		}
	}

	// Check for lowercase letters
	if p.RequireLowercase {
		hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
		if !hasLower {
			return errors.New("password must contain at least one lowercase letter")
		}
	}

	// Check for digits
	if p.RequireDigit {
		hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)
		if !hasDigit {
			return errors.New("password must contain at least one digit")
		}
	}

	// Check for special characters
	if p.RequireSpecial {
		hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(password)
		if !hasSpecial {
			return errors.New("password must contain at least one special character")
		}
	}

	// Check for repeated characters
	if p.MaxRepeatedChars > 0 {
		for i := 0; i <= len(password)-p.MaxRepeatedChars; i++ {
			char := password[i]
			repeated := true
			for j := 1; j < p.MaxRepeatedChars; j++ {
				if i+j >= len(password) || password[i+j] != char {
					repeated = false
					break
				}
			}
			if repeated {
				return errors.New("password contains too many repeated characters")
			}
		}
	}

	// Check if password contains username
	if p.DisallowUsername && username != "" && len(username) > 2 {
		if strings.Contains(strings.ToLower(password), strings.ToLower(username)) {
			return errors.New("password cannot contain your username")
		}
	}

	// Check if password contains email
	if p.DisallowEmail && email != "" {
		emailParts := strings.Split(email, "@")
		if len(emailParts) > 0 && len(emailParts[0]) > 2 {
			if strings.Contains(strings.ToLower(password), strings.ToLower(emailParts[0])) {
				return errors.New("password cannot contain your email address")
			}
		}
	}

	// Check for common sequences
	if p.DisallowSequences {
		commonSequences := []string{
			"123456", "12345", "123456789", "password", "qwerty", "abc123", "admin",
			"welcome", "monkey", "login", "passw0rd", "654321", "master", "hello",
			"freedom", "whatever", "qazwsx", "trustno1", "letmein", "dragon", "baseball",
			"football", "superman", "batman", "iloveyou", "starwars", "princess",
		}

		lowerPassword := strings.ToLower(password)
		for _, seq := range commonSequences {
			if strings.Contains(lowerPassword, seq) {
				return errors.New("password contains a common sequence")
			}
		}
	}

	// Check for common passwords
	if p.DisallowCommon {
		// This is a small subset of common passwords
		// In a real implementation, this would be a much larger list loaded from a file
		commonPasswords := map[string]bool{
			"password":   true,
			"123456":     true,
			"12345678":   true,
			"qwerty":     true,
			"abc123":     true,
			"password1":  true,
			"admin":      true,
			"welcome":    true,
			"monkey":     true,
			"login":      true,
			"passw0rd":   true,
			"654321":     true,
			"master":     true,
			"hello":      true,
			"freedom":    true,
			"whatever":   true,
			"qazwsx":     true,
			"trustno1":   true,
			"letmein":    true,
			"dragon":     true,
			"baseball":   true,
			"football":   true,
			"superman":   true,
			"batman":     true,
			"iloveyou":   true,
			"starwars":   true,
			"princess":   true,
			"sunshine":   true,
			"ashley":     true,
			"123123":     true,
			"1234":       true,
			"12345":      true,
			"123456789":  true,
			"1234567890": true,
			"adobe123":   true,
			"test":       true,
			"guest":      true,
			"user":       true,
			"unknown":    true,
			"computer":   true,
			"internet":   true,
			"shadow":     true,
			"michael":    true,
			"jennifer":   true,
		}

		if commonPasswords[strings.ToLower(password)] {
			return errors.New("password is too common")
		}
	}

	return nil
}

// PasswordStrength represents the strength of a password
type PasswordStrength int

const (
	VeryWeak PasswordStrength = iota
	Weak
	Moderate
	Strong
	VeryStrong
)

// EvaluatePasswordStrength evaluates the strength of a password
func EvaluatePasswordStrength(password string) PasswordStrength {
	score := 0

	// Length
	if len(password) >= 8 {
		score++
	}
	if len(password) >= 10 {
		score++
	}
	if len(password) >= 12 {
		score++
	}

	// Character types
	if regexp.MustCompile(`[A-Z]`).MatchString(password) {
		score++
	}
	if regexp.MustCompile(`[a-z]`).MatchString(password) {
		score++
	}
	if regexp.MustCompile(`[0-9]`).MatchString(password) {
		score++
	}
	if regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(password) {
		score++
	}

	// Variety
	uniqueChars := make(map[rune]bool)
	for _, char := range password {
		uniqueChars[char] = true
	}
	if len(uniqueChars) >= 8 {
		score++
	}
	if len(uniqueChars) >= 12 {
		score++
	}

	// Determine strength based on score
	switch {
	case score <= 3:
		return VeryWeak
	case score <= 5:
		return Weak
	case score <= 7:
		return Moderate
	case score <= 9:
		return Strong
	default:
		return VeryStrong
	}
}

// PasswordStrengthToString converts a password strength to a string
func PasswordStrengthToString(strength PasswordStrength) string {
	switch strength {
	case VeryWeak:
		return "Very Weak"
	case Weak:
		return "Weak"
	case Moderate:
		return "Moderate"
	case Strong:
		return "Strong"
	case VeryStrong:
		return "Very Strong"
	default:
		return "Unknown"
	}
}
