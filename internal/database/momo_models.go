package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MoMoTransaction represents a Mobile Money payment transaction
type MoMoTransaction struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	UserID          uuid.UUID `gorm:"type:uuid;not null"`
	TransactionID   string    `gorm:"type:varchar(255);not null;uniqueIndex"`
	ReferenceID     string    `gorm:"type:varchar(255);not null"`
	PhoneNumber     string    `gorm:"type:varchar(20);not null"`
	Amount          float64   `gorm:"type:decimal(20,2);not null"`
	Currency        string    `gorm:"type:varchar(3);not null;default:'GHS'"`
	Description     string    `gorm:"type:text"`
	Status          string    `gorm:"type:varchar(20);not null;default:'PENDING'"`
	FinancialID     string    `gorm:"type:varchar(255)"`
	Reason          string    `gorm:"type:text"`
	CallbackURL     string    `gorm:"type:varchar(255)"`
	CallbackStatus  string    `gorm:"type:varchar(20)"`
	CallbackTime    *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time `gorm:"index"`
}

// BeforeCreate will set a UUID rather than numeric ID.
func (m *MoMoTransaction) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// MoMoDisbursement represents a Mobile Money disbursement transaction
type MoMoDisbursement struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	UserID        uuid.UUID `gorm:"type:uuid;not null"`
	TransactionID string    `gorm:"type:varchar(255);not null;uniqueIndex"`
	ReferenceID   string    `gorm:"type:varchar(255);not null"`
	PhoneNumber   string    `gorm:"type:varchar(20);not null"`
	Amount        float64   `gorm:"type:decimal(20,2);not null"`
	Currency      string    `gorm:"type:varchar(3);not null;default:'GHS'"`
	Description   string    `gorm:"type:text"`
	Status        string    `gorm:"type:varchar(20);not null;default:'PENDING'"`
	FinancialID   string    `gorm:"type:varchar(255)"`
	Reason        string    `gorm:"type:text"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time `gorm:"index"`
}

// BeforeCreate will set a UUID rather than numeric ID.
func (m *MoMoDisbursement) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
