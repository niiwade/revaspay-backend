package security

import (
	"math"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"gorm.io/gorm"
)

// RiskFactor represents a risk factor that contributes to session risk score
type RiskFactor struct {
	Name        string
	Description string
	Weight      float64
	Evaluate    func(session *database.EnhancedSession, db *gorm.DB) float64
}

// RiskLevel represents the risk level of a session
type RiskLevel string

const (
	// RiskLevelLow indicates a low risk session
	RiskLevelLow RiskLevel = "low"
	// RiskLevelMedium indicates a medium risk session
	RiskLevelMedium RiskLevel = "medium"
	// RiskLevelHigh indicates a high risk session
	RiskLevelHigh RiskLevel = "high"
	// RiskLevelCritical indicates a critical risk session
	RiskLevelCritical RiskLevel = "critical"
)

// SessionRiskEvaluator evaluates the risk of a session
type SessionRiskEvaluator struct {
	db          *gorm.DB
	riskFactors []RiskFactor
}

// NewSessionRiskEvaluator creates a new session risk evaluator
func NewSessionRiskEvaluator(db *gorm.DB) *SessionRiskEvaluator {
	return &SessionRiskEvaluator{
		db:          db,
		riskFactors: defaultRiskFactors(),
	}
}

// EvaluateSession evaluates the risk of a session
func (e *SessionRiskEvaluator) EvaluateSession(sessionID uuid.UUID) (float64, RiskLevel, map[string]float64) {
	var session database.EnhancedSession
	if err := e.db.Where("id = ?", sessionID).First(&session).Error; err != nil {
		return 100, RiskLevelCritical, nil
	}

	return e.evaluateSessionRisk(&session)
}

// evaluateSessionRisk evaluates the risk of a session
func (e *SessionRiskEvaluator) evaluateSessionRisk(session *database.EnhancedSession) (float64, RiskLevel, map[string]float64) {
	factorScores := make(map[string]float64)
	totalScore := 0.0
	totalWeight := 0.0

	for _, factor := range e.riskFactors {
		score := factor.Evaluate(session, e.db)
		factorScores[factor.Name] = score
		totalScore += score * factor.Weight
		totalWeight += factor.Weight
	}

	// Normalize score to 0-100
	normalizedScore := (totalScore / totalWeight) * 100

	// Determine risk level
	riskLevel := getRiskLevel(normalizedScore)

	return normalizedScore, riskLevel, factorScores
}

// getRiskLevel determines the risk level based on the score
func getRiskLevel(score float64) RiskLevel {
	switch {
	case score < 25:
		return RiskLevelLow
	case score < 50:
		return RiskLevelMedium
	case score < 75:
		return RiskLevelHigh
	default:
		return RiskLevelCritical
	}
}

// defaultRiskFactors returns the default risk factors
func defaultRiskFactors() []RiskFactor {
	return []RiskFactor{
		{
			Name:        "location_change",
			Description: "Unusual location change",
			Weight:      1.5,
			Evaluate: func(session *database.EnhancedSession, db *gorm.DB) float64 {
				// Get user's previous sessions
				var prevSessions []database.EnhancedSession
				db.Where("user_id = ? AND id != ? AND created_at < ?", 
					session.UserID, session.ID, session.CreatedAt).
					Order("created_at DESC").
					Limit(5).
					Find(&prevSessions)

				if len(prevSessions) == 0 {
					return 0.5 // No history, moderate risk
				}

				// Check if current IP is in a different country/region than previous sessions
				currentIP := net.ParseIP(session.IPAddress)
				if currentIP == nil {
					return 0.7 // Invalid IP, higher risk
				}

				// Count how many previous sessions had different IPs
				differentIPs := 0
				for _, prevSession := range prevSessions {
					if prevSession.IPAddress != session.IPAddress {
						differentIPs++
					}
				}

				// Calculate risk based on IP diversity
				return float64(differentIPs) / float64(len(prevSessions))
			},
		},
		{
			Name:        "device_change",
			Description: "New or unusual device",
			Weight:      1.2,
			Evaluate: func(session *database.EnhancedSession, db *gorm.DB) float64 {
				// Check if device fingerprint has been seen before
				var count int64
				db.Model(&database.EnhancedSession{}).
					Where("user_id = ? AND device_fingerprint = ? AND id != ?", 
						session.UserID, session.DeviceFingerprint, session.ID).
					Count(&count)

				if count == 0 {
					return 0.8 // New device, higher risk
				}

				// Risk decreases with more sessions from this device
				return math.Max(0.1, 1.0-float64(count)/10.0)
			},
		},
		{
			Name:        "time_pattern",
			Description: "Unusual login time",
			Weight:      0.8,
			Evaluate: func(session *database.EnhancedSession, db *gorm.DB) float64 {
				// Get user's previous sessions
				var prevSessions []database.EnhancedSession
				db.Where("user_id = ? AND id != ?", session.UserID, session.ID).
					Order("created_at DESC").
					Limit(10).
					Find(&prevSessions)

				if len(prevSessions) < 3 {
					return 0.5 // Not enough history, moderate risk
				}

				// Check if current hour is unusual
				currentHour := session.CreatedAt.Hour()
				
				// Count sessions in similar hours
				similarTimeCount := 0
				for _, prevSession := range prevSessions {
					prevHour := prevSession.CreatedAt.Hour()
					if abs(prevHour-currentHour) <= 3 || abs(prevHour-currentHour) >= 21 {
						similarTimeCount++
					}
				}

				// Calculate risk based on time pattern
				return 1.0 - float64(similarTimeCount)/float64(len(prevSessions))
			},
		},
		{
			Name:        "failed_attempts",
			Description: "Recent failed login attempts",
			Weight:      2.0,
			Evaluate: func(session *database.EnhancedSession, db *gorm.DB) float64 {
				// Count recent failed login attempts
				var count int64
				db.Model(&database.FailedLoginAttempt{}).
					Where("user_id = ? AND created_at > ?", 
						session.UserID, session.CreatedAt.Add(-24*time.Hour)).
					Count(&count)

				// Risk increases with more failed attempts
				return math.Min(1.0, float64(count)/5.0)
			},
		},
		{
			Name:        "user_agent_anomaly",
			Description: "Unusual user agent",
			Weight:      1.0,
			Evaluate: func(session *database.EnhancedSession, db *gorm.DB) float64 {
				// Get user's previous sessions
				var prevSessions []database.EnhancedSession
				db.Where("user_id = ? AND id != ?", session.UserID, session.ID).
					Order("created_at DESC").
					Limit(5).
					Find(&prevSessions)

				if len(prevSessions) == 0 {
					return 0.5 // No history, moderate risk
				}

				// Check if current user agent is similar to previous ones
				currentUA := strings.ToLower(session.UserAgent)
				similarCount := 0
				
				for _, prevSession := range prevSessions {
					prevUA := strings.ToLower(prevSession.UserAgent)
					
					// Check for major browser/OS changes
					if (strings.Contains(currentUA, "chrome") && strings.Contains(prevUA, "chrome")) ||
						(strings.Contains(currentUA, "firefox") && strings.Contains(prevUA, "firefox")) ||
						(strings.Contains(currentUA, "safari") && strings.Contains(prevUA, "safari")) ||
						(strings.Contains(currentUA, "edge") && strings.Contains(prevUA, "edge")) {
						similarCount++
					}
					
					// Check OS consistency
					if (strings.Contains(currentUA, "windows") && strings.Contains(prevUA, "windows")) ||
						(strings.Contains(currentUA, "macintosh") && strings.Contains(prevUA, "macintosh")) ||
						(strings.Contains(currentUA, "linux") && strings.Contains(prevUA, "linux")) ||
						(strings.Contains(currentUA, "android") && strings.Contains(prevUA, "android")) ||
						(strings.Contains(currentUA, "iphone") && strings.Contains(prevUA, "iphone")) {
						similarCount++
					}
				}

				// Calculate risk based on user agent similarity
				return 1.0 - float64(similarCount)/(float64(len(prevSessions))*2.0)
			},
		},
		{
			Name:        "session_velocity",
			Description: "Unusual session creation velocity",
			Weight:      1.8,
			Evaluate: func(session *database.EnhancedSession, db *gorm.DB) float64 {
				// Find the most recent session before this one
				var prevSession database.EnhancedSession
				if err := db.Where("user_id = ? AND id != ? AND created_at < ?", 
					session.UserID, session.ID, session.CreatedAt).
					Order("created_at DESC").
					First(&prevSession).Error; err != nil {
					return 0.3 // No previous session, low-moderate risk
				}

				// Calculate time difference in hours
				timeDiff := session.CreatedAt.Sub(prevSession.CreatedAt).Hours()
				
				// If sessions are created too close together from different locations, higher risk
				if timeDiff < 1.0 && session.IPAddress != prevSession.IPAddress {
					// Calculate theoretical travel speed (km/h)
					// This is simplified and assumes IP geolocation is accurate
					return 0.9 // Very high risk for sessions minutes apart in different locations
				}
				
				// If time difference is very small (< 5 min) but same location, moderate risk
				if timeDiff < 0.08 { // Less than 5 minutes
					return 0.5
				}
				
				return 0.2 // Normal session velocity
			},
		},
	}
}

// abs returns the absolute value of x
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// UpdateSessionRiskScore updates the risk score of a session
func UpdateSessionRiskScore(db *gorm.DB, sessionID uuid.UUID) error {
	evaluator := NewSessionRiskEvaluator(db)
	score, riskLevel, _ := evaluator.EvaluateSession(sessionID)
	
	// Update session with risk score and level
	return db.Model(&database.EnhancedSession{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"risk_score": score,
			"risk_level": string(riskLevel),
		}).Error
}

// CheckSessionRisk checks if a session is risky and requires additional verification
func CheckSessionRisk(db *gorm.DB, sessionID uuid.UUID) (bool, RiskLevel) {
	var session database.EnhancedSession
	if err := db.Where("id = ?", sessionID).First(&session).Error; err != nil {
		return true, RiskLevelCritical
	}
	
	// If risk score is not calculated yet, calculate it
	if session.RiskScore == 0 {
		UpdateSessionRiskScore(db, sessionID)
		// Refresh session data
		db.Where("id = ?", sessionID).First(&session)
	}
	
	// Determine if additional verification is needed based on risk level
	needsVerification := session.RiskLevel == string(RiskLevelHigh) || 
		session.RiskLevel == string(RiskLevelCritical)
	
	return needsVerification, RiskLevel(session.RiskLevel)
}
