package security

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// RecoveryAttempt represents a failed recovery attempt
type RecoveryAttempt struct {
	Timestamp time.Time
	IP        string
	UserAgent string
}

// RecoveryProtection provides protection against brute force attacks on account recovery
type RecoveryProtection struct {
	// Map of email to failed attempts
	emailAttempts map[string][]RecoveryAttempt
	// Map of user ID to failed attempts
	userIDAttempts map[uuid.UUID][]RecoveryAttempt
	// Map of IP to failed attempts
	ipAttempts map[string][]RecoveryAttempt
	// Lock for concurrent access
	mu sync.RWMutex
	// Configuration
	config RecoveryProtectionConfig
}

// RecoveryProtectionConfig holds configuration for recovery protection
type RecoveryProtectionConfig struct {
	// Maximum number of failed attempts per email within window
	MaxAttemptsPerEmail int
	// Maximum number of failed attempts per user ID within window
	MaxAttemptsPerUserID int
	// Maximum number of failed attempts per IP within window
	MaxAttemptsPerIP int
	// Time window for counting attempts
	WindowDuration time.Duration
	// Lockout duration after exceeding max attempts
	LockoutDuration time.Duration
}

// DefaultRecoveryProtectionConfig returns the default configuration
func DefaultRecoveryProtectionConfig() RecoveryProtectionConfig {
	return RecoveryProtectionConfig{
		MaxAttemptsPerEmail: 5,
		MaxAttemptsPerUserID: 5,
		MaxAttemptsPerIP:    10,
		WindowDuration:      15 * time.Minute,
		LockoutDuration:     30 * time.Minute,
	}
}

// NewRecoveryProtection creates a new recovery protection instance
func NewRecoveryProtection(config RecoveryProtectionConfig) *RecoveryProtection {
	return &RecoveryProtection{
		emailAttempts:  make(map[string][]RecoveryAttempt),
		userIDAttempts: make(map[uuid.UUID][]RecoveryAttempt),
		ipAttempts:     make(map[string][]RecoveryAttempt),
		config:         config,
	}
}

// RecordFailedAttempt records a failed recovery attempt
func (rp *RecoveryProtection) RecordFailedAttempt(email string, userID *uuid.UUID, ip, userAgent string) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	now := time.Now()
	attempt := RecoveryAttempt{
		Timestamp: now,
		IP:        ip,
		UserAgent: userAgent,
	}

	// Record by email
	if email != "" {
		rp.emailAttempts[email] = append(rp.emailAttempts[email], attempt)
		rp.cleanupOldAttempts(email, nil, "")
	}

	// Record by user ID
	if userID != nil {
		rp.userIDAttempts[*userID] = append(rp.userIDAttempts[*userID], attempt)
		rp.cleanupOldAttempts("", userID, "")
	}

	// Record by IP
	if ip != "" {
		rp.ipAttempts[ip] = append(rp.ipAttempts[ip], attempt)
		rp.cleanupOldAttempts("", nil, ip)
	}
}

// IsBlocked checks if recovery attempts are blocked for the given identifiers
func (rp *RecoveryProtection) IsBlocked(email string, userID *uuid.UUID, ip string) (bool, time.Time) {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	now := time.Now()
	windowStart := now.Add(-rp.config.WindowDuration)

	// Check by email
	if email != "" {
		attempts := rp.countAttemptsInWindow(rp.emailAttempts[email], windowStart)
		if attempts >= rp.config.MaxAttemptsPerEmail {
			// Calculate when the lockout will expire
			lastAttempt := rp.getLastAttemptTime(rp.emailAttempts[email])
			lockoutEnd := lastAttempt.Add(rp.config.LockoutDuration)
			if now.Before(lockoutEnd) {
				return true, lockoutEnd
			}
		}
	}

	// Check by user ID
	if userID != nil {
		attempts := rp.countAttemptsInWindow(rp.userIDAttempts[*userID], windowStart)
		if attempts >= rp.config.MaxAttemptsPerUserID {
			lastAttempt := rp.getLastAttemptTime(rp.userIDAttempts[*userID])
			lockoutEnd := lastAttempt.Add(rp.config.LockoutDuration)
			if now.Before(lockoutEnd) {
				return true, lockoutEnd
			}
		}
	}

	// Check by IP
	if ip != "" {
		attempts := rp.countAttemptsInWindow(rp.ipAttempts[ip], windowStart)
		if attempts >= rp.config.MaxAttemptsPerIP {
			lastAttempt := rp.getLastAttemptTime(rp.ipAttempts[ip])
			lockoutEnd := lastAttempt.Add(rp.config.LockoutDuration)
			if now.Before(lockoutEnd) {
				return true, lockoutEnd
			}
		}
	}

	return false, time.Time{}
}

// countAttemptsInWindow counts the number of attempts within the window
func (rp *RecoveryProtection) countAttemptsInWindow(attempts []RecoveryAttempt, windowStart time.Time) int {
	count := 0
	for _, attempt := range attempts {
		if attempt.Timestamp.After(windowStart) {
			count++
		}
	}
	return count
}

// getLastAttemptTime gets the timestamp of the last attempt
func (rp *RecoveryProtection) getLastAttemptTime(attempts []RecoveryAttempt) time.Time {
	if len(attempts) == 0 {
		return time.Time{}
	}
	return attempts[len(attempts)-1].Timestamp
}

// cleanupOldAttempts removes attempts older than the window
func (rp *RecoveryProtection) cleanupOldAttempts(email string, userID *uuid.UUID, ip string) {
	cutoff := time.Now().Add(-rp.config.WindowDuration)

	// Cleanup email attempts
	if email != "" {
		var newAttempts []RecoveryAttempt
		for _, attempt := range rp.emailAttempts[email] {
			if attempt.Timestamp.After(cutoff) {
				newAttempts = append(newAttempts, attempt)
			}
		}
		rp.emailAttempts[email] = newAttempts
	}

	// Cleanup user ID attempts
	if userID != nil {
		var newAttempts []RecoveryAttempt
		for _, attempt := range rp.userIDAttempts[*userID] {
			if attempt.Timestamp.After(cutoff) {
				newAttempts = append(newAttempts, attempt)
			}
		}
		rp.userIDAttempts[*userID] = newAttempts
	}

	// Cleanup IP attempts
	if ip != "" {
		var newAttempts []RecoveryAttempt
		for _, attempt := range rp.ipAttempts[ip] {
			if attempt.Timestamp.After(cutoff) {
				newAttempts = append(newAttempts, attempt)
			}
		}
		rp.ipAttempts[ip] = newAttempts
	}
}
