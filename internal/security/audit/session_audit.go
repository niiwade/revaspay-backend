package audit

import (
	"github.com/google/uuid"
)

// EventTypeSecurity is for security-related events
const EventTypeSecurity EventType = "security"

// GetAuditLogsForSession gets audit logs for a specific session
func (l *Logger) GetAuditLogsForSession(sessionID uuid.UUID, limit int) ([]AuditLog, error) {
	var logs []AuditLog

	if err := l.db.Where("target_id = ?", sessionID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, err
	}

	return logs, nil
}
