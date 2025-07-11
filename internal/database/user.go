package database

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// User methods and functions are defined here
// The User struct itself is defined in models.go

// BeforeCreate is called before creating a new user
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

// SetPassword sets the password for a user
func (u *User) SetPassword(password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hashedPassword)
	return nil
}

// CheckPassword checks if the provided password is correct
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}

// CreateUser creates a new user
func CreateUser(db *gorm.DB, email, password, firstName, lastName string) (*User, error) {
	user := &User{
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
	}

	if err := user.SetPassword(password); err != nil {
		return nil, err
	}

	if err := db.Create(user).Error; err != nil {
		return nil, err
	}

	return user, nil
}

// FindUserByEmail finds a user by email
func FindUserByEmail(db *gorm.DB, email string) (*User, error) {
	var user User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindUserByID finds a user by ID
func FindUserByID(db *gorm.DB, id uuid.UUID) (*User, error) {
	var user User
	if err := db.Where("id = ?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateLastLogin updates the last login time for a user
func UpdateLastLogin(db *gorm.DB, userID uuid.UUID) error {
	now := time.Now()
	return db.Model(&User{}).Where("id = ?", userID).Update("last_login_at", now).Error
}

// UpdateUserProfile updates a user's profile information
func UpdateUserProfile(db *gorm.DB, userID uuid.UUID, updates map[string]interface{}) error {
	return db.Model(&User{}).Where("id = ?", userID).Updates(updates).Error
}
