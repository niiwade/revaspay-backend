package database

import (
	"time"

	"github.com/google/uuid"
)

// KYCStatus represents the status of a KYC verification
type KYCStatus string

// KYC status constants
const (
	KYCStatusNotSubmitted KYCStatus = "not_submitted"
	KYCStatusPending      KYCStatus = "pending"
	KYCStatusApproved     KYCStatus = "approved"
	KYCStatusRejected     KYCStatus = "rejected"
)

// DocumentType represents the type of ID document
type DocumentType string

// Document type constants
const (
	DocumentTypePassport       DocumentType = "passport"
	DocumentTypeIDCard         DocumentType = "id_card"
	DocumentTypeDriversLicense DocumentType = "drivers_license"
)

// KYCHistory represents a record of KYC status changes
type KYCHistory struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	KYCID          uuid.UUID `gorm:"type:uuid;index" json:"kyc_id"`
	KYC            KYC       `gorm:"foreignKey:KYCID" json:"-"`
	PreviousStatus KYCStatus `json:"previous_status"`
	NewStatus      KYCStatus `json:"new_status"`
	Comment        string    `json:"comment"`
	ChangedBy      uuid.UUID `gorm:"type:uuid" json:"changed_by"`
	CreatedAt      time.Time `json:"created_at"`
}
