package security

import (
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"gorm.io/gorm"
)

// DetailedRiskFactor represents a specific security risk factor with description
type DetailedRiskFactor struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Weight      int    `json:"weight"` // 1-10, higher is riskier
}

// DetailedRiskAssessment contains the detailed risk assessment for a login attempt
type DetailedRiskAssessment struct {
	Score        int                `json:"score"`        // 0-100, higher is riskier
	Factors      []DetailedRiskFactor `json:"factors"`      // Factors contributing to risk
	Action       string             `json:"action"`       // allow, challenge, block
	RequireMFA   bool               `json:"require_mfa"`  // Whether MFA should be required
	TrustExpiry  *time.Time         `json:"trust_expiry"` // When trust expires for this assessment
	AssessmentID string             `json:"assessment_id"`
}

// DetailedRiskAssessor evaluates login risk based on various factors
type DetailedRiskAssessor struct {
	db                *gorm.DB
	knownIPThreshold  int           // Number of days to consider an IP as "known"
	locationThreshold int           // Distance threshold in km to consider a location change suspicious
	timeThreshold     time.Duration // Time threshold to consider login time suspicious
	highRiskCountries []string      // List of high-risk countries
}

// NewDetailedRiskAssessor creates a new detailed risk assessor
func NewDetailedRiskAssessor(db *gorm.DB) *DetailedRiskAssessor {
	return &DetailedRiskAssessor{
		db:                db,
		knownIPThreshold:  30,  // 30 days
		locationThreshold: 500, // 500 km
		timeThreshold:     6 * time.Hour,
		highRiskCountries: []string{"XX", "YY", "ZZ"}, // Replace with actual high-risk country codes
	}
}

// AssessLoginRisk assesses the risk of a login attempt
func (r *DetailedRiskAssessor) AssessLoginRisk(userID uuid.UUID, ipAddress, userAgent string) (*DetailedRiskAssessment, error) {
	assessment := &DetailedRiskAssessment{
		Score:        0,
		Factors:      []DetailedRiskFactor{},
		Action:       "allow",
		RequireMFA:   false,
		AssessmentID: uuid.New().String(),
	}

	// Get user's previous sessions
	var sessions []database.EnhancedSession
	if err := r.db.Where("user_id = ? AND status = ?", userID, database.SessionStatusActive).
		Order("created_at DESC").
		Limit(10).
		Find(&sessions).Error; err != nil {
		// If we can't get previous sessions, consider it higher risk
		assessment.Score += 20
		assessment.Factors = append(assessment.Factors, DetailedRiskFactor{
			Type:        "no_history",
			Description: "No login history available",
			Weight:      5,
		})
	}

	// Check if this is the first login
	if len(sessions) == 0 {
		assessment.Score += 10
		assessment.Factors = append(assessment.Factors, DetailedRiskFactor{
			Type:        "first_login",
			Description: "First login for this user",
			Weight:      3,
		})
		assessment.RequireMFA = true
	} else {
		// Check if IP address is known
		knownIP := false
		for _, session := range sessions {
			if session.IPAddress == ipAddress {
				knownIP = true
				break
			}
		}

		if !knownIP {
			assessment.Score += 25
			assessment.Factors = append(assessment.Factors, DetailedRiskFactor{
				Type:        "unknown_ip",
				Description: "Login from new IP address",
				Weight:      6,
			})
		}

		// Check if user agent is different
		if len(sessions) > 0 && sessions[0].UserAgent != userAgent {
			assessment.Score += 15
			assessment.Factors = append(assessment.Factors, DetailedRiskFactor{
				Type:        "new_device",
				Description: "Login from new device or browser",
				Weight:      4,
			})
		}
	}

	// Check if IP is from a high-risk country
	country := r.getCountryFromIP(ipAddress)
	if r.isHighRiskCountry(country) {
		assessment.Score += 30
		assessment.Factors = append(assessment.Factors, DetailedRiskFactor{
			Type:        "high_risk_country",
			Description: "Login from high-risk country",
			Weight:      8,
		})
	}

	// Check for suspicious login time
	if r.isSuspiciousLoginTime(userID, time.Now()) {
		assessment.Score += 15
		assessment.Factors = append(assessment.Factors, DetailedRiskFactor{
			Type:        "unusual_time",
			Description: "Login at unusual time",
			Weight:      4,
		})
	}

	// Determine action based on risk score
	if assessment.Score >= 80 {
		assessment.Action = "block"
	} else if assessment.Score >= 50 {
		assessment.Action = "challenge"
		assessment.RequireMFA = true
	} else if assessment.Score >= 30 {
		assessment.Action = "allow"
		assessment.RequireMFA = true
	} else {
		assessment.Action = "allow"
	}

	return assessment, nil
}

// RecordSuccessfulLogin records a successful login for future risk assessment
func (r *DetailedRiskAssessor) RecordSuccessfulLogin(userID uuid.UUID, sessionID uuid.UUID, ipAddress, userAgent string) error {
	// This is handled by the session creation, but we could add additional logic here
	return nil
}

// RecordFailedLogin records a failed login attempt
func (r *DetailedRiskAssessor) RecordFailedLogin(userID *uuid.UUID, ipAddress, userAgent, reason string) error {
	failedLogin := struct {
		ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
		UserID    *uuid.UUID `gorm:"type:uuid"`
		IPAddress string
		UserAgent string
		Reason    string
		CreatedAt time.Time
	}{
		UserID:    userID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Reason:    reason,
		CreatedAt: time.Now(),
	}

	return r.db.Table("failed_logins").Create(&failedLogin).Error
}

// CheckBruteForceAttempts checks if there are too many failed login attempts
func (r *DetailedRiskAssessor) CheckBruteForceAttempts(userID *uuid.UUID, ipAddress string) (bool, error) {
	var count int64

	// Check by user ID if available
	if userID != nil {
		if err := r.db.Table("failed_logins").
			Where("user_id = ? AND created_at > ?", userID, time.Now().Add(-30*time.Minute)).
			Count(&count).Error; err != nil {
			return false, err
		}

		if count >= 5 {
			return true, nil
		}
	}

	// Check by IP address
	if err := r.db.Table("failed_logins").
		Where("ip_address = ? AND created_at > ?", ipAddress, time.Now().Add(-30*time.Minute)).
		Count(&count).Error; err != nil {
		return false, err
	}

	return count >= 10, nil
}

// UpdateSessionRiskMetadata updates the risk metadata for a session
func (r *DetailedRiskAssessor) UpdateSessionRiskMetadata(sessionID uuid.UUID, assessment *DetailedRiskAssessment) error {
	var session database.EnhancedSession
	if err := r.db.Where("id = ?", sessionID).First(&session).Error; err != nil {
		return err
	}

	// Get metadata
	metadata, err := session.GetMetadata()
	if err != nil {
		// If we can't parse existing metadata, create new one
		metadata = &database.SessionMetadata{}
	}

	// Update risk information
	metadata.RiskScore = assessment.Score
	metadata.RiskFactors = []string{}
	for _, factor := range assessment.Factors {
		metadata.RiskFactors = append(metadata.RiskFactors, factor.Type)
	}

	// Set updated metadata
	if err := session.SetMetadata(metadata); err != nil {
		return err
	}

	// Save to database
	return r.db.Model(&database.EnhancedSession{}).
		Where("id = ?", sessionID).
		Update("metadata_json", session.MetadataJSON).Error
}

// Helper methods

// getCountryFromIP gets the country code from an IP address
func (r *DetailedRiskAssessor) getCountryFromIP(ipAddress string) string {
	// In a real implementation, this would use a GeoIP database
	// For now, just return a placeholder
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		return "UNKNOWN"
	}

	// Check if it's a private IP
	if ip.IsPrivate() || ip.IsLoopback() {
		return "LOCAL"
	}

	// Simplified example - in production use a proper GeoIP database
	if strings.HasPrefix(ipAddress, "41.") {
		return "NG" // Nigeria
	} else if strings.HasPrefix(ipAddress, "196.") {
		return "GH" // Ghana
	}

	return "UNKNOWN"
}

// isHighRiskCountry checks if a country is considered high risk
func (r *DetailedRiskAssessor) isHighRiskCountry(countryCode string) bool {
	for _, code := range r.highRiskCountries {
		if code == countryCode {
			return true
		}
	}
	return false
}

// isSuspiciousLoginTime checks if the login time is suspicious
func (r *DetailedRiskAssessor) isSuspiciousLoginTime(userID uuid.UUID, loginTime time.Time) bool {
	// Get user's typical login times
	var sessions []database.EnhancedSession
	if err := r.db.Where("user_id = ? AND created_at > ?", userID, time.Now().AddDate(0, -1, 0)).
		Order("created_at DESC").
		Find(&sessions).Error; err != nil {
		return false
	}

	if len(sessions) < 3 {
		return false
	}

	// Check if current hour is within typical login hours
	loginHour := loginTime.Hour()
	typicalHours := make(map[int]int)

	for _, session := range sessions {
		hour := session.CreatedAt.Hour()
		typicalHours[hour]++
	}

	// If this hour has less than 10% of logins, consider it suspicious
	threshold := len(sessions) / 10
	if threshold < 1 {
		threshold = 1
	}

	return typicalHours[loginHour] < threshold
}
