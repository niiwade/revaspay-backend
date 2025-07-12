package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// KYCStatus represents the status of a user's KYC verification
type KYCStatus string

const (
	KYCStatusPending    KYCStatus = "pending"
	KYCStatusInProgress KYCStatus = "in_progress"
	KYCStatusApproved   KYCStatus = "approved"
	KYCStatusRejected   KYCStatus = "rejected"
	KYCStatusExpired    KYCStatus = "expired"
)

// DocumentType represents the type of document uploaded for KYC
type DocumentType string

const (
	DocumentTypeID      DocumentType = "id"
	DocumentTypePassport DocumentType = "passport"
	DocumentTypeLicense  DocumentType = "license"
	DocumentTypeSelfie   DocumentType = "selfie"
)

// KYCVerification represents a KYC verification record
type KYCVerification struct {
	ID             uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID         uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	User           User           `gorm:"foreignKey:UserID" json:"-"`
	Status         KYCStatus      `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	SessionID      string         `gorm:"type:varchar(255)" json:"session_id"`
	WorkflowID     string         `gorm:"type:varchar(255)" json:"workflow_id"`
	VerificationURL string        `gorm:"type:text" json:"verification_url"`
	IDDocType      *DocumentType  `gorm:"type:varchar(50)" json:"id_doc_type"`
	IDDocNumber    *string        `gorm:"type:varchar(100)" json:"id_doc_number"`
	IDDocCountry   *string        `gorm:"type:varchar(2)" json:"id_doc_country"`
	IDDocExpiry    *time.Time     `json:"id_doc_expiry"`
	FullName       *string        `gorm:"type:varchar(255)" json:"full_name"`
	DateOfBirth    *time.Time     `json:"date_of_birth"`
	Address        *string        `gorm:"type:text" json:"address"`
	ReportURL      *string        `gorm:"type:text" json:"report_url"`
	AdminNotes     *string        `gorm:"type:text" json:"admin_notes"`
	RejectionReason *string       `gorm:"type:text" json:"rejection_reason"`
	CreatedAt      time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// KYCDocument represents a document uploaded for KYC verification
type KYCDocument struct {
	ID             uuid.UUID    `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	VerificationID uuid.UUID    `gorm:"type:uuid;not null" json:"verification_id"`
	Verification   KYCVerification `gorm:"foreignKey:VerificationID" json:"-"`
	Type           DocumentType `gorm:"type:varchar(50);not null" json:"type"`
	FilePath       string       `gorm:"type:text;not null" json:"file_path"`
	FileName       string       `gorm:"type:varchar(255);not null" json:"file_name"`
	FileSize       int64        `json:"file_size"`
	MimeType       string       `gorm:"type:varchar(100)" json:"mime_type"`
	UploadedAt     time.Time    `gorm:"default:CURRENT_TIMESTAMP" json:"uploaded_at"`
	DiditDocID     *string      `gorm:"type:varchar(255)" json:"didit_doc_id"`
}

// KYCVerificationHistory tracks the history of status changes for a KYC verification
type KYCVerificationHistory struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	VerificationID uuid.UUID `gorm:"type:uuid;not null" json:"verification_id"`
	Verification   KYCVerification `gorm:"foreignKey:VerificationID" json:"-"`
	PreviousStatus KYCStatus `gorm:"type:varchar(20);not null" json:"previous_status"`
	NewStatus      KYCStatus `gorm:"type:varchar(20);not null" json:"new_status"`
	ChangedBy      uuid.UUID `gorm:"type:uuid" json:"changed_by"`
	ChangedByUser  User      `gorm:"foreignKey:ChangedBy" json:"-"`
	Notes          *string   `gorm:"type:text" json:"notes"`
	CreatedAt      time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
}
