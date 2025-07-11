package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MFAMethod represents the type of MFA method
type MFAMethod string

const (
	MFAMethodTOTP   MFAMethod = "TOTP"
	MFAMethodSMS    MFAMethod = "SMS"
	MFAMethodEmail  MFAMethod = "EMAIL"
	MFAMethodBackup MFAMethod = "BACKUP"
)

// MFASettings stores a user's MFA configuration
type MFASettings struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID         uuid.UUID  `gorm:"type:uuid;uniqueIndex" json:"user_id"`
	User           User       `gorm:"foreignKey:UserID" json:"-"`
	Enabled        bool       `json:"enabled"`
	DefaultMethod  MFAMethod  `json:"default_method"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastVerifiedAt *time.Time `json:"last_verified_at"`
}

// MFADevice represents an MFA device or method configured for a user
type MFADevice struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID       uuid.UUID  `gorm:"type:uuid" json:"user_id"`
	User         User       `gorm:"foreignKey:UserID" json:"-"`
	MFASettingsID uuid.UUID  `gorm:"type:uuid" json:"mfa_settings_id"`
	MFASettings   MFASettings `gorm:"foreignKey:MFASettingsID" json:"-"`
	Name         string     `json:"name"`
	Method       MFAMethod  `json:"method"`
	Secret       string     `json:"-"` // Encrypted secret
	PhoneNumber  *string    `json:"phone_number,omitempty"`
	Email        *string    `json:"email,omitempty"`
	Verified     bool       `json:"verified"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// MFABackupCode represents a backup code for MFA recovery
type MFABackupCode struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID       uuid.UUID `gorm:"type:uuid" json:"user_id"`
	User         User      `gorm:"foreignKey:UserID" json:"-"`
	MFASettingsID uuid.UUID `gorm:"type:uuid" json:"mfa_settings_id"`
	MFASettings   MFASettings `gorm:"foreignKey:MFASettingsID" json:"-"`
	Code         string    `json:"-"` // Hashed code
	Used         bool      `json:"used"`
	UsedAt       *time.Time `json:"used_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// MFARecoveryToken represents a token for MFA recovery
type MFARecoveryToken struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID       uuid.UUID  `gorm:"type:uuid" json:"user_id"`
	User         User       `gorm:"foreignKey:UserID" json:"-"`
	Token        string     `json:"-"` // Hashed token
	ExpiresAt    time.Time  `json:"expires_at"`
	Used         bool       `json:"used"`
	UsedAt       *time.Time `json:"used_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// GetMFASettings gets a user's MFA settings
func GetMFASettings(db *gorm.DB, userID uuid.UUID) (*MFASettings, error) {
	var settings MFASettings
	result := db.Where("user_id = ?", userID).First(&settings)
	
	if result.Error == gorm.ErrRecordNotFound {
		// Create default settings if not found
		settings = MFASettings{
			UserID:        userID,
			Enabled:       false,
			DefaultMethod: MFAMethodTOTP,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		
		if err := db.Create(&settings).Error; err != nil {
			return nil, err
		}
		
		return &settings, nil
	}
	
	if result.Error != nil {
		return nil, result.Error
	}
	
	return &settings, nil
}

// GetMFADevices gets all MFA devices for a user
func GetMFADevices(db *gorm.DB, userID uuid.UUID) ([]MFADevice, error) {
	var devices []MFADevice
	
	if err := db.Where("user_id = ?", userID).Find(&devices).Error; err != nil {
		return nil, err
	}
	
	return devices, nil
}

// GetMFADevice gets a specific MFA device
func GetMFADevice(db *gorm.DB, deviceID uuid.UUID) (*MFADevice, error) {
	var device MFADevice
	
	if err := db.Where("id = ?", deviceID).First(&device).Error; err != nil {
		return nil, err
	}
	
	return &device, nil
}

// CreateMFADevice creates a new MFA device
func CreateMFADevice(db *gorm.DB, device *MFADevice) error {
	return db.Create(device).Error
}

// UpdateMFADevice updates an MFA device
func UpdateMFADevice(db *gorm.DB, device *MFADevice) error {
	device.UpdatedAt = time.Now()
	return db.Save(device).Error
}

// DeleteMFADevice deletes an MFA device
func DeleteMFADevice(db *gorm.DB, deviceID uuid.UUID) error {
	return db.Delete(&MFADevice{}, deviceID).Error
}

// GetUnusedBackupCodes gets unused backup codes for a user
func GetUnusedBackupCodes(db *gorm.DB, userID uuid.UUID) ([]MFABackupCode, error) {
	var codes []MFABackupCode
	
	if err := db.Where("user_id = ? AND used = ?", userID, false).Find(&codes).Error; err != nil {
		return nil, err
	}
	
	return codes, nil
}

// CreateBackupCodes creates backup codes for a user
func CreateBackupCodes(db *gorm.DB, userID uuid.UUID, settingsID uuid.UUID, hashedCodes []string) error {
	// Begin transaction
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	
	// Delete existing unused backup codes
	if err := tx.Where("user_id = ? AND used = ?", userID, false).Delete(&MFABackupCode{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// Create new backup codes
	now := time.Now()
	for _, code := range hashedCodes {
		backupCode := MFABackupCode{
			UserID:       userID,
			MFASettingsID: settingsID,
			Code:         code,
			Used:         false,
			CreatedAt:    now,
		}
		
		if err := tx.Create(&backupCode).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	
	// Commit transaction
	return tx.Commit().Error
}

// UseBackupCode marks a backup code as used
func UseBackupCode(db *gorm.DB, userID uuid.UUID, hashedCode string) (bool, error) {
	var code MFABackupCode
	
	// Find the code
	result := db.Where("user_id = ? AND code = ? AND used = ?", userID, hashedCode, false).First(&code)
	if result.Error == gorm.ErrRecordNotFound {
		return false, nil
	}
	if result.Error != nil {
		return false, result.Error
	}
	
	// Mark as used
	now := time.Now()
	code.Used = true
	code.UsedAt = &now
	
	if err := db.Save(&code).Error; err != nil {
		return false, err
	}
	
	return true, nil
}

// CreateRecoveryToken creates a recovery token for MFA reset
func CreateRecoveryToken(db *gorm.DB, userID uuid.UUID, hashedToken string, expiryHours int) (*MFARecoveryToken, error) {
	// Invalidate existing tokens
	if err := db.Model(&MFARecoveryToken{}).
		Where("user_id = ? AND used = ?", userID, false).
		Update("used", true).Error; err != nil {
		return nil, err
	}
	
	// Create new token
	token := MFARecoveryToken{
		UserID:    userID,
		Token:     hashedToken,
		ExpiresAt: time.Now().Add(time.Duration(expiryHours) * time.Hour),
		Used:      false,
		CreatedAt: time.Now(),
	}
	
	if err := db.Create(&token).Error; err != nil {
		return nil, err
	}
	
	return &token, nil
}

// ValidateRecoveryToken validates a recovery token
func ValidateRecoveryToken(db *gorm.DB, userID uuid.UUID, hashedToken string) (bool, error) {
	var token MFARecoveryToken
	
	// Find the token
	result := db.Where("user_id = ? AND token = ? AND used = ? AND expires_at > ?", 
		userID, hashedToken, false, time.Now()).First(&token)
	
	if result.Error == gorm.ErrRecordNotFound {
		return false, nil
	}
	if result.Error != nil {
		return false, result.Error
	}
	
	// Mark as used
	now := time.Now()
	token.Used = true
	token.UsedAt = &now
	
	if err := db.Save(&token).Error; err != nil {
		return false, err
	}
	
	return true, nil
}

// DisableMFA disables MFA for a user
func DisableMFA(db *gorm.DB, userID uuid.UUID) error {
	// Begin transaction
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	
	// Update MFA settings
	if err := tx.Model(&MFASettings{}).
		Where("user_id = ?", userID).
		Update("enabled", false).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// Update user record
	if err := tx.Model(&User{}).
		Where("id = ?", userID).
		Update("two_factor_enabled", false).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// Commit transaction
	return tx.Commit().Error
}

// EnableMFA enables MFA for a user
func EnableMFA(db *gorm.DB, userID uuid.UUID, defaultMethod MFAMethod) error {
	// Begin transaction
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	
	// Update MFA settings
	now := time.Now()
	if err := tx.Model(&MFASettings{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"enabled":          true,
			"default_method":   defaultMethod,
			"last_verified_at": now,
			"updated_at":       now,
		}).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// Update user record
	if err := tx.Model(&User{}).
		Where("id = ?", userID).
		Update("two_factor_enabled", true).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// Commit transaction
	return tx.Commit().Error
}
