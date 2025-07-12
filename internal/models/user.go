package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	Email         string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	Username      string         `gorm:"type:varchar(50);uniqueIndex" json:"username"`
	FirstName     string         `gorm:"type:varchar(100)" json:"first_name"`
	LastName      string         `gorm:"type:varchar(100)" json:"last_name"`
	PasswordHash  string         `gorm:"type:varchar(255);not null" json:"-"`
	IsVerified    bool           `gorm:"default:false" json:"is_verified"`
	IsActive      bool           `gorm:"default:true" json:"is_active"`
	IsAdmin       bool           `gorm:"default:false" json:"is_admin"`
	PhoneNumber   *string        `gorm:"type:varchar(20)" json:"phone_number"`
	CountryCode   *string        `gorm:"type:varchar(5)" json:"country_code"`
	ProfileImage  *string        `gorm:"type:text" json:"profile_image"`
	LastLoginAt   *time.Time     `json:"last_login_at"`
	CreatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}
