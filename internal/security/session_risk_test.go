package security

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/revaspay/backend/internal/database"
)

func TestSessionRiskEvaluator(t *testing.T) {
	// Create a mock DB for testing
	db := &gorm.DB{}
	
	// Create a session risk evaluator
	evaluator := NewSessionRiskEvaluator(db)

	// Create a test session
	sessionID := uuid.New()
	userID := uuid.New()
	session := database.EnhancedSession{
		ID:        sessionID,
		UserID:    userID,
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		IPAddress: "192.168.1.1",
		CreatedAt: time.Now().Add(-24 * time.Hour),
	}

	// Test with no risk factors
	riskScore, riskLevel, riskFactors := evaluator.evaluateSessionRisk(&session)
	assert.LessOrEqual(t, riskScore, 100.0, "Risk score should be within range")
	assert.NotEmpty(t, riskLevel, "Risk level should not be empty")
	assert.NotNil(t, riskFactors, "Risk factors map should not be nil")

	// Test with location change
	// In a real test, we would set up the database with previous sessions
	// that have different locations
	
	// Test with device change
	// In a real test, we would set up the database with previous sessions
	// that have different devices
}

func TestGetRiskLevel(t *testing.T) {
	// Test different risk scores
	assert.Equal(t, RiskLevelLow, getRiskLevel(0), "0 should be low risk")
	assert.Equal(t, RiskLevelLow, getRiskLevel(10), "10 should be low risk")
	assert.Equal(t, RiskLevelLow, getRiskLevel(24.9), "24.9 should be low risk")
	
	assert.Equal(t, RiskLevelMedium, getRiskLevel(25), "25 should be medium risk")
	assert.Equal(t, RiskLevelMedium, getRiskLevel(35), "35 should be medium risk")
	assert.Equal(t, RiskLevelMedium, getRiskLevel(49.9), "49.9 should be medium risk")
	
	assert.Equal(t, RiskLevelHigh, getRiskLevel(50), "50 should be high risk")
	assert.Equal(t, RiskLevelHigh, getRiskLevel(65), "65 should be high risk")
	assert.Equal(t, RiskLevelHigh, getRiskLevel(74.9), "74.9 should be high risk")
	
	assert.Equal(t, RiskLevelCritical, getRiskLevel(75), "75 should be critical risk")
	assert.Equal(t, RiskLevelCritical, getRiskLevel(85), "85 should be critical risk")
	assert.Equal(t, RiskLevelCritical, getRiskLevel(100), "100 should be critical risk")
}

func TestUpdateSessionRiskScore(t *testing.T) {
	// Create a mock DB for testing
	db := &gorm.DB{}
	
	// Create a test session
	sessionID := uuid.New()
	
	// This would normally fail in a real test without proper DB mocking
	// Just testing the function signature is correct
	err := UpdateSessionRiskScore(db, sessionID)
	assert.Error(t, err, "Should return error with mock DB")
}

func TestCheckSessionRisk(t *testing.T) {
	// Create a mock DB for testing
	db := &gorm.DB{}
	
	// Create a test session
	sessionID := uuid.New()
	
	// This would normally fail in a real test without proper DB mocking
	// Just testing the function signature is correct
	needsVerification, riskLevel := CheckSessionRisk(db, sessionID)
	assert.True(t, needsVerification, "Should return true with mock DB")
	assert.Equal(t, RiskLevelCritical, riskLevel, "Should return critical risk level with mock DB")
}
