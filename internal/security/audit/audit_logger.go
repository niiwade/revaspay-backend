package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EventType represents the type of audit event
type EventType string

// EventSeverity represents the severity level of an audit event
type EventSeverity string

const (
	// Event types
	EventTypeAuth            EventType = "auth"
	EventTypeSession         EventType = "session"
	EventTypeMFA             EventType = "mfa"
	EventTypeProfile         EventType = "profile"
	EventTypePayment         EventType = "payment"
	EventTypeAdmin           EventType = "admin"
	EventTypeEmailVerification EventType = "email_verification"
	
	// Severity levels
	SeverityInfo     EventSeverity = "info"
	SeverityWarning  EventSeverity = "warning"
	SeverityError    EventSeverity = "error"
	SeverityCritical EventSeverity = "critical"
)

// AuditLog represents an audit log entry in the database
type AuditLog struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	UserID      *uuid.UUID     `gorm:"type:uuid;index"`
	TargetID    *uuid.UUID     `gorm:"type:uuid;index"`
	EventType   string         `gorm:"index"`
	Severity    string         
	Description string         
	IPAddress   string         
	UserAgent   string         
	Metadata    string         // JSON string of additional data
	CreatedAt   time.Time      `gorm:"index"`
	Success     bool           `gorm:"index"`
}

// Logger is the audit logger
type Logger struct {
	db *gorm.DB
}

// NewLogger creates a new audit logger
func NewLogger(db *gorm.DB) *Logger {
	return &Logger{
		db: db,
	}
}

// Log logs an audit event
func (l *Logger) Log(eventType EventType, metadata map[string]interface{}) error {
	return l.LogWithContext(context.Background(), eventType, SeverityInfo, "", nil, nil, "", "", true, metadata)
}

// LogEvent logs an audit event with all details
func (l *Logger) LogEvent(c *gin.Context, eventType EventType, severity EventSeverity, 
	description string, userID, targetID *uuid.UUID, ipAddress, userAgent string, 
	success bool, metadata map[string]interface{}) error {
	
	return l.LogWithContext(c, eventType, severity, description, userID, targetID, ipAddress, userAgent, success, metadata)
}

// LogWithContext logs an audit event with context
func (l *Logger) LogWithContext(c context.Context, eventType EventType, severity EventSeverity, 
	description string, userID, targetID *uuid.UUID, ipAddress, userAgent string, 
	success bool, metadata map[string]interface{}) error {
	
	// Extract user ID from gin context if available and not explicitly provided
	if userID == nil {
		if gc, ok := c.(*gin.Context); ok {
			if id, exists := gc.Get("user_id"); exists {
				if idStr, ok := id.(string); ok {
					if parsedID, err := uuid.Parse(idStr); err == nil {
						userID = &parsedID
					}
				} else if parsedID, ok := id.(uuid.UUID); ok {
					userID = &parsedID
				}
			}
		}
	}
	
	// Extract IP and user agent from gin context if available and not explicitly provided
	if gc, ok := c.(*gin.Context); ok {
		if ipAddress == "" {
			ipAddress = gc.ClientIP()
		}
		if userAgent == "" {
			userAgent = gc.GetHeader("User-Agent")
		}
	}
	
	// Convert metadata to JSON
	var metadataJSON string
	if metadata != nil {
		metadataBytes, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		metadataJSON = string(metadataBytes)
	}
	
	// Create audit log entry
	auditLog := AuditLog{
		UserID:      userID,
		TargetID:    targetID,
		EventType:   string(eventType),
		Severity:    string(severity),
		Description: description,
		IPAddress:   ipAddress,
		UserAgent:   userAgent,
		Metadata:    metadataJSON,
		CreatedAt:   time.Now(),
		Success:     success,
	}
	
	// Save to database
	return l.db.Create(&auditLog).Error
}

// GetUserLogs gets audit logs for a specific user
func (l *Logger) GetUserLogs(userID uuid.UUID, limit, offset int) ([]AuditLog, error) {
	var logs []AuditLog
	err := l.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs).Error
	return logs, err
}

// GetSecurityLogs gets security-related audit logs
func (l *Logger) GetSecurityLogs(limit, offset int) ([]AuditLog, error) {
	var logs []AuditLog
	err := l.db.Where("event_type IN ? OR severity IN ?", 
		[]string{string(EventTypeAuth), string(EventTypeMFA), string(EventTypeSession)},
		[]string{string(SeverityWarning), string(SeverityError), string(SeverityCritical)}).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs).Error
	return logs, err
}
