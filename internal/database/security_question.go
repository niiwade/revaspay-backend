package database

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SecurityQuestion represents a security question for account recovery
type SecurityQuestion struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	Question  string         `gorm:"not null" json:"question"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// UserSecurityQuestion represents a user's answer to a security question
type UserSecurityQuestion struct {
	ID                uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID            uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	SecurityQuestionID uuid.UUID     `gorm:"type:uuid;not null" json:"security_question_id"`
	SecurityQuestion  SecurityQuestion `gorm:"foreignKey:SecurityQuestionID" json:"security_question"`
	AnswerHash        string         `gorm:"not null" json:"-"` // Hashed answer
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate is called before creating a new security question
func (sq *SecurityQuestion) BeforeCreate(tx *gorm.DB) error {
	if sq.ID == uuid.Nil {
		sq.ID = uuid.New()
	}
	return nil
}

// BeforeCreate is called before creating a new user security question
func (usq *UserSecurityQuestion) BeforeCreate(tx *gorm.DB) error {
	if usq.ID == uuid.Nil {
		usq.ID = uuid.New()
	}
	return nil
}

// HashAnswer hashes a security question answer
func HashAnswer(answer string) string {
	hash := sha256.Sum256([]byte(answer))
	return hex.EncodeToString(hash[:])
}

// SetAnswer sets the hashed answer for a security question
func (usq *UserSecurityQuestion) SetAnswer(answer string) {
	usq.AnswerHash = HashAnswer(answer)
}

// VerifyAnswer verifies if the provided answer matches the stored hash
func (usq *UserSecurityQuestion) VerifyAnswer(answer string) bool {
	return usq.AnswerHash == HashAnswer(answer)
}

// GetSecurityQuestions gets all available security questions
func GetSecurityQuestions(db *gorm.DB) ([]SecurityQuestion, error) {
	var questions []SecurityQuestion
	if err := db.Find(&questions).Error; err != nil {
		return nil, err
	}
	return questions, nil
}

// GetUserSecurityQuestions gets all security questions for a user
func GetUserSecurityQuestions(db *gorm.DB, userID uuid.UUID) ([]UserSecurityQuestion, error) {
	var questions []UserSecurityQuestion
	if err := db.Preload("SecurityQuestion").Where("user_id = ?", userID).Find(&questions).Error; err != nil {
		return nil, err
	}
	return questions, nil
}

// CreateUserSecurityQuestion creates a new security question answer for a user
func CreateUserSecurityQuestion(db *gorm.DB, userID, questionID uuid.UUID, answer string) (*UserSecurityQuestion, error) {
	userQuestion := &UserSecurityQuestion{
		UserID:            userID,
		SecurityQuestionID: questionID,
	}
	userQuestion.SetAnswer(answer)

	if err := db.Create(userQuestion).Error; err != nil {
		return nil, err
	}

	// Load the question
	if err := db.Preload("SecurityQuestion").First(userQuestion, "id = ?", userQuestion.ID).Error; err != nil {
		return nil, err
	}

	return userQuestion, nil
}

// UpdateUserSecurityQuestionAnswer updates a user's security question answer
func UpdateUserSecurityQuestionAnswer(db *gorm.DB, userQuestionID uuid.UUID, answer string) error {
	var userQuestion UserSecurityQuestion
	if err := db.First(&userQuestion, "id = ?", userQuestionID).Error; err != nil {
		return err
	}

	userQuestion.SetAnswer(answer)
	return db.Save(&userQuestion).Error
}

// DeleteUserSecurityQuestion deletes a user's security question
func DeleteUserSecurityQuestion(db *gorm.DB, userQuestionID uuid.UUID) error {
	return db.Delete(&UserSecurityQuestion{}, "id = ?", userQuestionID).Error
}

// VerifyUserSecurityQuestions verifies a user's security question answers
// Returns true if all provided answers are correct
func VerifyUserSecurityQuestions(db *gorm.DB, userID uuid.UUID, questionAnswers map[uuid.UUID]string) (bool, error) {
	var questions []UserSecurityQuestion
	if err := db.Where("user_id = ?", userID).Find(&questions).Error; err != nil {
		return false, err
	}

	// Check if we have enough questions to verify
	if len(questionAnswers) == 0 || len(questions) == 0 {
		return false, nil
	}

	// Check each provided answer
	correctAnswers := 0
	for _, question := range questions {
		if answer, exists := questionAnswers[question.SecurityQuestionID]; exists {
			if question.VerifyAnswer(answer) {
				correctAnswers++
			}
		}
	}

	// All provided answers must be correct and we need at least 2 correct answers
	return correctAnswers >= 2 && correctAnswers == len(questionAnswers), nil
}

// InitializeDefaultSecurityQuestions creates default security questions if none exist
func InitializeDefaultSecurityQuestions(db *gorm.DB) error {
	var count int64
	db.Model(&SecurityQuestion{}).Count(&count)
	
	if count > 0 {
		return nil // Questions already exist
	}

	defaultQuestions := []SecurityQuestion{
		{Question: "What was the name of your first pet?"},
		{Question: "What is your mother's maiden name?"},
		{Question: "What was the name of your primary school?"},
		{Question: "In what city were you born?"},
		{Question: "What was your childhood nickname?"},
		{Question: "What is the name of your favorite childhood friend?"},
		{Question: "What is your favorite movie?"},
		{Question: "What was your first car?"},
		{Question: "What is your favorite color?"},
		{Question: "What is the name of the street you grew up on?"},
	}

	// Create each question individually since CreateInBatch is not available
	for _, question := range defaultQuestions {
		if err := db.Create(&question).Error; err != nil {
			return err
		}
	}

	return nil
}
