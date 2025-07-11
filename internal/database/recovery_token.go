package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RecoveryToken represents a token for account recovery
type RecoveryToken struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	Token     string         `gorm:"uniqueIndex;not null" json:"token"`
	ExpiresAt time.Time      `gorm:"not null" json:"expires_at"`
	Used      bool           `gorm:"default:false" json:"used"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate is called before creating a new recovery token
func (rt *RecoveryToken) BeforeCreate(tx *gorm.DB) error {
	if rt.ID == uuid.Nil {
		rt.ID = uuid.New()
	}
	return nil
}

// CreateAccountRecoveryToken creates a new recovery token for a user
func CreateAccountRecoveryToken(db *gorm.DB, userID uuid.UUID, token string, expiresAt time.Time) (*RecoveryToken, error) {
	recoveryToken := &RecoveryToken{
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		Used:      false,
	}

	if err := db.Create(recoveryToken).Error; err != nil {
		return nil, err
	}

	return recoveryToken, nil
}

// GetRecoveryTokenByToken gets a recovery token by its token string
func GetRecoveryTokenByToken(db *gorm.DB, token string) (*RecoveryToken, error) {
	var recoveryToken RecoveryToken
	if err := db.Where("token = ? AND used = ?", token, false).First(&recoveryToken).Error; err != nil {
		return nil, err
	}

	return &recoveryToken, nil
}

// MarkRecoveryTokenAsUsed marks a recovery token as used
func MarkRecoveryTokenAsUsed(db *gorm.DB, tokenID uuid.UUID) error {
	return db.Model(&RecoveryToken{}).Where("id = ?", tokenID).Update("used", true).Error
}
