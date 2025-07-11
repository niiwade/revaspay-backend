package security

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRecoveryProtection(t *testing.T) {
	// Create a recovery protection instance with test configuration
	config := RecoveryProtectionConfig{
		MaxAttemptsPerEmail:    3,
		MaxAttemptsPerUserID:   3,
		MaxAttemptsPerIP:       5,
		WindowDuration:         30 * time.Minute,
		LockoutDuration:        15 * time.Minute,
	}
	protection := NewRecoveryProtection(config)

	// Test recording attempts for email
	email := "test@example.com"
	// User ID for testing
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000123")
	ip := "192.168.1.1"

	// Record attempts and check if blocked
	blocked, _ := protection.IsBlocked(email, nil, "")
	assert.False(t, blocked, "Email should not be blocked initially")
	
	// Record first attempt
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	blocked, _ = protection.IsBlocked(email, nil, "")
	assert.False(t, blocked, "Email should not be blocked after 1 attempt")
	
	// Record second attempt
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	blocked, _ = protection.IsBlocked(email, nil, "")
	assert.False(t, blocked, "Email should not be blocked after 2 attempts")
	
	// Record third attempt - should now be blocked
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	blocked, _ = protection.IsBlocked(email, nil, "")
	assert.True(t, blocked, "Email should be blocked after 3 attempts")

	// Test user ID blocking
	newEmail := "another@example.com"
	blocked, _ = protection.IsBlocked("", &userID, "")
	assert.False(t, blocked, "UserID should not be blocked initially")
	
	// Record attempts for new email but same user ID
	protection.RecordFailedAttempt(newEmail, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(newEmail, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(newEmail, &userID, ip, "Mozilla/5.0")
	
	blocked, _ = protection.IsBlocked("", &userID, "")
	assert.True(t, blocked, "UserID should be blocked after 3 attempts")

	// Test IP blocking
	newUserIDUUID := uuid.MustParse("00000000-0000-0000-0000-000000000456")
	newEmail2 := "third@example.com"
	
	// Record 4 attempts for new user and email but same IP
	protection.RecordFailedAttempt(newEmail2, &newUserIDUUID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(newEmail2, &newUserIDUUID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(newEmail2, &newUserIDUUID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(newEmail2, &newUserIDUUID, ip, "Mozilla/5.0")
	
	blocked, _ = protection.IsBlocked("", nil, ip)
	assert.False(t, blocked, "IP should not be blocked after 4 attempts")
	
	// Record fifth attempt - should now be blocked
	protection.RecordFailedAttempt(newEmail2, &newUserIDUUID, ip, "Mozilla/5.0")
	blocked, _ = protection.IsBlocked("", nil, ip)
	assert.True(t, blocked, "IP should be blocked after 5 attempts")

	// Test cleanup of old attempts
	// Create a new protection instance
	protection = NewRecoveryProtection(config)
	
	// Add some attempts
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	
	// Manually modify the timestamp of the first attempt to be outside the window
	for i, attempt := range protection.emailAttempts[email] {
		if i == 0 {
			attempt.Timestamp = time.Now().Add(-31 * time.Minute)
			protection.emailAttempts[email][i] = attempt
		}
	}
	
	// Clean up old attempts
	protection.cleanupOldAttempts(email, nil, "")
	
	// Should only have one attempt left for the email
	assert.Equal(t, 1, len(protection.emailAttempts[email]), "Should have 1 attempt left after cleanup")
	
	// Test reset functions
	protection = NewRecoveryProtection(config)
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	
	blocked, _ = protection.IsBlocked(email, nil, "")
	assert.True(t, blocked, "Email should be blocked")
	
	// Reset email by clearing the attempts
	protection.emailAttempts[email] = []RecoveryAttempt{}
	blocked, _ = protection.IsBlocked(email, nil, "")
	assert.False(t, blocked, "Email should not be blocked after reset")
	
	// Test reset user ID
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	
	blocked, _ = protection.IsBlocked("", &userID, "")
	assert.True(t, blocked, "UserID should be blocked")
	
	// Reset user ID by clearing the attempts
	protection.userIDAttempts[userID] = []RecoveryAttempt{}
	blocked, _ = protection.IsBlocked("", &userID, "")
	assert.False(t, blocked, "UserID should not be blocked after reset")
	
	// Test reset IP
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	
	blocked, _ = protection.IsBlocked("", nil, ip)
	assert.True(t, blocked, "IP should be blocked")
	
	// Reset IP by clearing the attempts
	protection.ipAttempts[ip] = []RecoveryAttempt{}
	blocked, _ = protection.IsBlocked("", nil, ip)
	assert.False(t, blocked, "IP should not be blocked after reset")
}

func TestRecoveryProtectionConcurrentAccess(t *testing.T) {
	// Create a recovery protection instance with test configuration
	config := RecoveryProtectionConfig{
		MaxAttemptsPerEmail:    3,
		MaxAttemptsPerUserID:   3,
		MaxAttemptsPerIP:       5,
		WindowDuration:         30 * time.Minute,
		LockoutDuration:        15 * time.Minute,
	}
	protection := NewRecoveryProtection(config)

	// Test concurrent access to the protection instance
	email := "test@example.com"
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000123")
	ip := "192.168.1.1"
	
	// Create channels to synchronize goroutines
	done := make(chan bool)
	
	// Start multiple goroutines to record attempts
	for i := 0; i < 10; i++ {
		go func() {
			protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
			done <- true
		}()
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Check that the attempts were recorded correctly
	blocked, _ := protection.IsBlocked(email, nil, "")
	assert.True(t, blocked, "Email should be blocked after concurrent attempts")
	
	blocked, _ = protection.IsBlocked("", &userID, "")
	assert.True(t, blocked, "UserID should be blocked after concurrent attempts")
	
	blocked, _ = protection.IsBlocked("", nil, ip)
	assert.True(t, blocked, "IP should be blocked after concurrent attempts")
	
	// Check the actual counts
	assert.Equal(t, 10, len(protection.emailAttempts[email]), "Should have recorded 10 attempts for email")
	assert.Equal(t, 10, len(protection.userIDAttempts[userID]), "Should have recorded 10 attempts for userID")
	assert.Equal(t, 10, len(protection.ipAttempts[ip]), "Should have recorded 10 attempts for IP")
}

func TestRecoveryProtectionLockoutDuration(t *testing.T) {
	// Create a recovery protection instance with test configuration
	config := RecoveryProtectionConfig{
		MaxAttemptsPerEmail:    3,
		MaxAttemptsPerUserID:   3,
		MaxAttemptsPerIP:       5,
		WindowDuration:         30 * time.Minute,
		LockoutDuration:        15 * time.Minute,
	}
	protection := NewRecoveryProtection(config)

	email := "test@example.com"
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000123")
	ip := "192.168.1.1"
	
	// Record enough attempts to trigger a block
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	protection.RecordFailedAttempt(email, &userID, ip, "Mozilla/5.0")
	
	// Verify the block is in place
	blocked, _ := protection.IsBlocked(email, nil, "")
	assert.True(t, blocked, "Email should be blocked")
	
	// Manually modify the timestamp of all attempts to be outside the lockout duration
	for i := range protection.emailAttempts[email] {
		protection.emailAttempts[email][i].Timestamp = time.Now().Add(-16 * time.Minute)
	}
	
	// Now the email should no longer be blocked due to lockout duration
	blocked, _ = protection.IsBlocked(email, nil, "")
	assert.False(t, blocked, "Email should not be blocked after lockout duration")
	
	// But the attempts should still be counted within the window
	assert.Equal(t, 3, len(protection.emailAttempts[email]), "Should still have 3 attempts recorded")
	
	// Manually modify the timestamp of all attempts to be outside the window
	for i := range protection.emailAttempts[email] {
		protection.emailAttempts[email][i].Timestamp = time.Now().Add(-31 * time.Minute)
	}
	
	// Clean up old attempts
	protection.cleanupOldAttempts(email, nil, "")
	
	// Should have no attempts left for the email
	assert.Equal(t, 0, len(protection.emailAttempts[email]), "Should have 0 attempts left after cleanup")
}
