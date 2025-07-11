package security

import (
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"gorm.io/gorm"
)

// RiskAssessment represents the result of a risk assessment
type RiskAssessment struct {
	AssessmentID string
	Score        float64
	Action       string // "allow", "challenge", "block"
	RequireMFA   bool
	Factors      map[string]float64
}

// RiskAssessor handles risk assessment for login attempts
type RiskAssessor struct {
	db *gorm.DB
}

// NewRiskAssessor creates a new risk assessor
func NewRiskAssessor(db *gorm.DB) *RiskAssessor {
	return &RiskAssessor{
		db: db,
	}
}

// AssessLoginRisk assesses the risk of a login attempt
func (r *RiskAssessor) AssessLoginRisk(userID uuid.UUID, ipAddress, userAgent string) (*RiskAssessment, error) {
	// Create a new assessment
	assessment := &RiskAssessment{
		AssessmentID: uuid.New().String(),
		Score:        0,
		Action:       "allow",
		RequireMFA:   false,
		Factors:      make(map[string]float64),
	}

	// Get user's previous sessions
	var sessions []database.EnhancedSession
	if err := r.db.Where("user_id = ?", userID).Order("created_at desc").Limit(10).Find(&sessions).Error; err != nil {
		return assessment, err
	}

	// Check if this is a new device/location
	newDevice := true
	newLocation := true
	unusualTime := false

	if len(sessions) > 0 {
		// Check if device has been seen before
		for _, session := range sessions {
			if session.DeviceFingerprint != "" && session.UserAgent == userAgent {
				newDevice = false
				break
			}
		}

		// Check if IP has been seen before
		for _, session := range sessions {
			if session.IPAddress == ipAddress {
				newLocation = false
				break
			}
		}

		// Check if login time is unusual
		if len(sessions) >= 3 {
			// Calculate average login hour
			var totalHour int
			for i := 0; i < 3; i++ {
				totalHour += sessions[i].CreatedAt.Hour()
			}
			avgHour := totalHour / 3

			// Check if current hour is significantly different
			currentHour := time.Now().Hour()
			if absInt(currentHour-avgHour) > 6 {
				unusualTime = true
			}
		}
	}

	// Calculate risk score based on factors
	if newDevice {
		assessment.Score += 30
		assessment.Factors["new_device"] = 30
	}

	if newLocation {
		assessment.Score += 20
		assessment.Factors["new_location"] = 20
	}

	if unusualTime {
		assessment.Score += 15
		assessment.Factors["unusual_time"] = 15
	}

	// Check for failed login attempts
	var failedAttempts int64
	r.db.Model(&database.LoginAttempt{}).
		Where("user_id = ? AND success = ? AND created_at > ?", userID, false, time.Now().Add(-24*time.Hour)).
		Count(&failedAttempts)

	if failedAttempts > 0 {
		failedScore := float64(failedAttempts) * 10
		if failedScore > 40 {
			failedScore = 40
		}
		assessment.Score += failedScore
		assessment.Factors["failed_attempts"] = failedScore
	}

	// Determine action based on risk score
	if assessment.Score >= 70 {
		assessment.Action = "block"
		assessment.RequireMFA = true
	} else if assessment.Score >= 40 {
		assessment.Action = "challenge"
		assessment.RequireMFA = true
	} else if assessment.Score >= 20 {
		assessment.RequireMFA = true
	}

	return assessment, nil
}

// RecordSuccessfulLogin records a successful login
func (r *RiskAssessor) RecordSuccessfulLogin(userID, sessionID uuid.UUID, ipAddress, userAgent string) error {
	// Create login attempt record
	attempt := database.LoginAttempt{
		UserID:    userID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Success:   true,
		SessionID: &sessionID,
		CreatedAt: time.Now(),
	}

	return r.db.Create(&attempt).Error
}

// UpdateSessionRiskMetadata updates session metadata with risk assessment
func (r *RiskAssessor) UpdateSessionRiskMetadata(sessionID uuid.UUID, assessment *RiskAssessment) error {
	// Get session
	var session database.EnhancedSession
	if err := r.db.Where("id = ?", sessionID).First(&session).Error; err != nil {
		return err
	}

	// Get metadata
	metadata, err := session.GetMetadata()
	if err != nil {
		return err
	}

	// Update metadata with risk assessment
	metadata.RiskScore = int(assessment.Score)
	metadata.RiskFactors = make([]string, 0)
	for factor := range assessment.Factors {
		metadata.RiskFactors = append(metadata.RiskFactors, factor)
	}

	// Save metadata
	if err := session.SetMetadata(metadata); err != nil {
		return err
	}

	// Update session
	return r.db.Save(&session).Error
}

// absInt returns the absolute value of an integer
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
