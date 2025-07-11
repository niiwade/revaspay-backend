package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SecurityQuestionAnswer represents a security question and answer pair
type SecurityQuestionAnswer struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID     uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	User       User      `gorm:"foreignKey:UserID" json:"-"`
	Question   string    `json:"question"`
	Answer     string    `json:"-"` // Hashed answer
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	VerifiedAt *time.Time `json:"verified_at"`
}

// GetUserSecurityQuestionAnswers gets security question answers for a user
func GetUserSecurityQuestionAnswers(db *gorm.DB, userID uuid.UUID) ([]SecurityQuestionAnswer, error) {
	var questions []SecurityQuestionAnswer
	
	if err := db.Where("user_id = ?", userID).Find(&questions).Error; err != nil {
		return nil, err
	}
	
	return questions, nil
}

// SaveSecurityQuestion saves a security question and answer
func SaveSecurityQuestion(db *gorm.DB, question *SecurityQuestionAnswer) error {
	if question.ID == uuid.Nil {
		question.CreatedAt = time.Now()
	}
	question.UpdatedAt = time.Now()
	
	return db.Save(question).Error
}

// VerifySecurityQuestionAnswer verifies a security question answer
func VerifySecurityQuestionAnswer(db *gorm.DB, questionID uuid.UUID, hashedAnswer string) (bool, error) {
	var question SecurityQuestionAnswer
	
	if err := db.Where("id = ?", questionID).First(&question).Error; err != nil {
		return false, err
	}
	
	// Compare hashed answers
	if question.Answer != hashedAnswer {
		return false, nil
	}
	
	// Update verification timestamp
	now := time.Now()
	question.VerifiedAt = &now
	
	if err := db.Save(&question).Error; err != nil {
		return true, err
	}
	
	return true, nil
}
