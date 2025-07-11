package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ComplianceReport represents a compliance report for international payments
type ComplianceReport struct {
	ID                    uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID                uuid.UUID      `gorm:"type:uuid" json:"user_id"`
	InternationalPaymentID uuid.UUID     `gorm:"type:uuid" json:"international_payment_id"`
	ReportType            string         `json:"report_type"` // kyc, aml, transaction
	ReportData            string         `json:"report_data"` // JSON string with report data
	Status                string         `json:"status"`      // generated, submitted, approved, rejected
	SubmittedToAuthority  bool           `gorm:"default:false" json:"submitted_to_authority"`
	AuthorityReference    string         `json:"authority_reference"`
	Notes                 string         `json:"notes"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	SubmittedAt           *time.Time     `json:"submitted_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
}

// ComplianceCheck represents a compliance check result
type ComplianceCheck struct {
	CheckName string `json:"check_name"`
	Passed    bool   `json:"passed"`
	Message   string `json:"message"`
}
