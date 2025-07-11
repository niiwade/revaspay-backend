package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuditEventType represents the type of security event
type AuditEventType string

// Define audit event types
const (
	AuditEventLogin                AuditEventType = "LOGIN"
	AuditEventLoginFailed          AuditEventType = "LOGIN_FAILED"
	AuditEventLogout               AuditEventType = "LOGOUT"
	AuditEventPasswordChange       AuditEventType = "PASSWORD_CHANGE"
	AuditEventPasswordReset        AuditEventType = "PASSWORD_RESET"
	AuditEventPasswordResetRequest AuditEventType = "PASSWORD_RESET_REQUEST"
	AuditEventEmailChange          AuditEventType = "EMAIL_CHANGE"
	AuditEventProfileUpdate        AuditEventType = "PROFILE_UPDATE"
	AuditEventAccountLocked        AuditEventType = "ACCOUNT_LOCKED"
	AuditEventAccountUnlocked      AuditEventType = "ACCOUNT_UNLOCKED"
	AuditEventMFAEnabled           AuditEventType = "MFA_ENABLED"
	AuditEventMFADisabled          AuditEventType = "MFA_DISABLED"
	AuditEventMFAFailed            AuditEventType = "MFA_FAILED"
	AuditEventSessionCreated       AuditEventType = "SESSION_CREATED"
	AuditEventSessionRevoked       AuditEventType = "SESSION_REVOKED"
	AuditEventAllSessionsRevoked   AuditEventType = "ALL_SESSIONS_REVOKED"
	AuditEventAdminAction          AuditEventType = "ADMIN_ACTION"
	AuditEventPermissionChange     AuditEventType = "PERMISSION_CHANGE"
	AuditEventRoleChange           AuditEventType = "ROLE_CHANGE"
	AuditEventAPIKeyCreated        AuditEventType = "API_KEY_CREATED"
	AuditEventAPIKeyRevoked        AuditEventType = "API_KEY_REVOKED"
	AuditEventUserCreated          AuditEventType = "USER_CREATED"
	AuditEventUserDeleted          AuditEventType = "USER_DELETED"
	AuditEventUserSuspended        AuditEventType = "USER_SUSPENDED"
	AuditEventUserReinstated       AuditEventType = "USER_REINSTATED"
)

// AuditEventSeverity represents the severity level of an audit event
type AuditEventSeverity string

// Define audit event severity levels
const (
	AuditSeverityInfo    AuditEventSeverity = "INFO"
	AuditSeverityWarning AuditEventSeverity = "WARNING"
	AuditSeverityError   AuditEventSeverity = "ERROR"
	AuditSeverityCritical AuditEventSeverity = "CRITICAL"
)

// AuditLog represents a security audit log entry
type AuditLog struct {
	ID          uuid.UUID         `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	UserID      *uuid.UUID        `gorm:"type:uuid" json:"user_id"`
	IPAddress   string            `json:"ip_address"`
	UserAgent   string            `json:"user_agent"`
	EventType   AuditEventType    `json:"event_type"`
	Severity    AuditEventSeverity `json:"severity"`
	Description string            `json:"description"`
	Details     string            `gorm:"type:text" json:"details"` // JSON string of additional details
	Success     bool              `json:"success"`
	SessionID   *uuid.UUID        `gorm:"type:uuid" json:"session_id"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// AuditLogger handles security audit logging
type AuditLogger struct {
	db *gorm.DB
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(db *gorm.DB) *AuditLogger {
	return &AuditLogger{
		db: db,
	}
}

// LogEvent logs a security event
func (a *AuditLogger) LogEvent(ctx context.Context, eventType AuditEventType, severity AuditEventSeverity, description string, userID *uuid.UUID, sessionID *uuid.UUID, ipAddress, userAgent string, success bool, details map[string]interface{}) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("failed to marshal audit log details: %w", err)
	}

	auditLog := AuditLog{
		Timestamp:   time.Now(),
		UserID:      userID,
		IPAddress:   ipAddress,
		UserAgent:   userAgent,
		EventType:   eventType,
		Severity:    severity,
		Description: description,
		Details:     string(detailsJSON),
		Success:     success,
		SessionID:   sessionID,
	}

	if err := a.db.Create(&auditLog).Error; err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	return nil
}

// LogLoginAttempt logs a login attempt
func (a *AuditLogger) LogLoginAttempt(ctx context.Context, userID *uuid.UUID, ipAddress, userAgent, email string, success bool, reason string) error {
	eventType := AuditEventLogin
	severity := AuditSeverityInfo
	description := "Successful login"
	
	if !success {
		eventType = AuditEventLoginFailed
		severity = AuditSeverityWarning
		description = "Failed login attempt"
	}

	details := map[string]interface{}{
		"email":  email,
		"reason": reason,
	}

	return a.LogEvent(ctx, eventType, severity, description, userID, nil, ipAddress, userAgent, success, details)
}

// LogPasswordChange logs a password change event
func (a *AuditLogger) LogPasswordChange(ctx context.Context, userID uuid.UUID, ipAddress, userAgent string, success bool, reason string) error {
	eventType := AuditEventPasswordChange
	severity := AuditSeverityInfo
	description := "Password changed successfully"
	
	if !success {
		severity = AuditSeverityWarning
		description = "Failed password change attempt"
	}

	details := map[string]interface{}{
		"reason": reason,
	}

	return a.LogEvent(ctx, eventType, severity, description, &userID, nil, ipAddress, userAgent, success, details)
}

// LogPasswordReset logs a password reset event
func (a *AuditLogger) LogPasswordReset(ctx context.Context, userID uuid.UUID, ipAddress, userAgent string, success bool, reason string) error {
	eventType := AuditEventPasswordReset
	severity := AuditSeverityInfo
	description := "Password reset successfully"
	
	if !success {
		severity = AuditSeverityWarning
		description = "Failed password reset attempt"
	}

	details := map[string]interface{}{
		"reason": reason,
	}

	return a.LogEvent(ctx, eventType, severity, description, &userID, nil, ipAddress, userAgent, success, details)
}

// LogPasswordResetRequest logs a password reset request event
func (a *AuditLogger) LogPasswordResetRequest(ctx context.Context, userID *uuid.UUID, ipAddress, userAgent, email string, success bool, reason string) error {
	eventType := AuditEventPasswordResetRequest
	severity := AuditSeverityInfo
	description := "Password reset requested"
	
	if !success {
		severity = AuditSeverityWarning
		description = "Failed password reset request"
	}

	details := map[string]interface{}{
		"email":  email,
		"reason": reason,
	}

	return a.LogEvent(ctx, eventType, severity, description, userID, nil, ipAddress, userAgent, success, details)
}

// LogSessionActivity logs session-related events
func (a *AuditLogger) LogSessionActivity(ctx context.Context, eventType AuditEventType, userID uuid.UUID, sessionID *uuid.UUID, ipAddress, userAgent string, success bool, details map[string]interface{}) error {
	severity := AuditSeverityInfo
	description := string(eventType)
	
	if !success {
		severity = AuditSeverityWarning
		description = "Failed " + string(eventType)
	}

	return a.LogEvent(ctx, eventType, severity, description, &userID, sessionID, ipAddress, userAgent, success, details)
}

// LogAdminAction logs administrative actions
func (a *AuditLogger) LogAdminAction(ctx context.Context, adminID uuid.UUID, targetUserID *uuid.UUID, ipAddress, userAgent, action string, success bool, details map[string]interface{}) error {
	eventType := AuditEventAdminAction
	severity := AuditSeverityInfo
	description := "Admin action: " + action
	
	if !success {
		severity = AuditSeverityWarning
		description = "Failed admin action: " + action
	}

	if details == nil {
		details = make(map[string]interface{})
	}
	details["admin_id"] = adminID.String()
	if targetUserID != nil {
		details["target_user_id"] = targetUserID.String()
	}
	details["action"] = action

	return a.LogEvent(ctx, eventType, severity, description, &adminID, nil, ipAddress, userAgent, success, details)
}

// QueryAuditLogs queries audit logs with filters
func (a *AuditLogger) QueryAuditLogs(userID *uuid.UUID, eventTypes []AuditEventType, startTime, endTime *time.Time, ipAddress string, limit, offset int) ([]AuditLog, int64, error) {
	var logs []AuditLog
	var count int64
	
	query := a.db.Model(&AuditLog{})
	
	if userID != nil {
		query = query.Where("user_id = ?", userID)
	}
	
	if len(eventTypes) > 0 {
		query = query.Where("event_type IN ?", eventTypes)
	}
	
	if startTime != nil {
		query = query.Where("timestamp >= ?", startTime)
	}
	
	if endTime != nil {
		query = query.Where("timestamp <= ?", endTime)
	}
	
	if ipAddress != "" {
		query = query.Where("ip_address = ?", ipAddress)
	}
	
	// Get total count
	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	
	// Get paginated results
	if err := query.Order("timestamp DESC").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	
	return logs, count, nil
}

// GetUserAuditLogs gets audit logs for a specific user
func (a *AuditLogger) GetUserAuditLogs(userID uuid.UUID, limit, offset int) ([]AuditLog, int64, error) {
	return a.QueryAuditLogs(&userID, nil, nil, nil, "", limit, offset)
}

// GetRecentSecurityEvents gets recent security events for a user
func (a *AuditLogger) GetRecentSecurityEvents(userID uuid.UUID, days int) ([]AuditLog, error) {
	var logs []AuditLog
	
	startTime := time.Now().AddDate(0, 0, -days)
	
	securityEvents := []AuditEventType{
		AuditEventLogin,
		AuditEventLoginFailed,
		AuditEventPasswordChange,
		AuditEventPasswordReset,
		AuditEventPasswordResetRequest,
		AuditEventEmailChange,
		AuditEventMFAEnabled,
		AuditEventMFADisabled,
		AuditEventSessionCreated,
		AuditEventSessionRevoked,
		AuditEventAllSessionsRevoked,
	}
	
	if err := a.db.Where("user_id = ? AND event_type IN ? AND timestamp >= ?", 
		userID, securityEvents, startTime).
		Order("timestamp DESC").
		Find(&logs).Error; err != nil {
		return nil, err
	}
	
	return logs, nil
}
