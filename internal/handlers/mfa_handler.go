package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/utils"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// MFAHandler handles multi-factor authentication operations
type MFAHandler struct {
	db         *gorm.DB
	auditLogger *utils.AuditLogger
	mfaConfig  utils.MFAConfig
}

// NewMFAHandler creates a new MFA handler
func NewMFAHandler(db *gorm.DB, auditLogger *utils.AuditLogger) *MFAHandler {
	return &MFAHandler{
		db:         db,
		auditLogger: auditLogger,
		mfaConfig:  utils.DefaultMFAConfig(),
	}
}

// SetupTOTP initiates TOTP setup for a user
func (h *MFAHandler) SetupTOTP(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	uid, err := uuid.Parse(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get user details
	var user database.User
	if err := h.db.First(&user, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Get or create MFA settings
	settings, err := database.GetMFASettings(h.db, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get MFA settings"})
		return
	}

	// Generate TOTP key
	accountName := user.Email
	key, err := utils.GenerateTOTPKey(h.mfaConfig, accountName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate TOTP key"})
		return
	}

	// Store the secret temporarily in the session
	// In a real implementation, you'd encrypt this before storing
	c.SetCookie("mfa_setup_secret", key.Secret, 600, "/", "", true, true)

	// Hash backup codes for storage
	var hashedBackupCodes []string
	for _, code := range key.BackupCodes {
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash backup codes"})
			return
		}
		hashedBackupCodes = append(hashedBackupCodes, string(hash))
	}

	// Store backup codes in the database
	if err := database.CreateBackupCodes(h.db, uid, settings.ID, hashedBackupCodes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store backup codes"})
		return
	}

	// Log the event
	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	h.auditLogger.LogEvent(c, utils.AuditEventMFAEnabled, utils.AuditSeverityInfo, 
		"MFA setup initiated", &uid, nil, ipAddress, userAgent, true, 
		map[string]interface{}{"method": "TOTP"})

	// Return setup information
	c.JSON(http.StatusOK, gin.H{
		"secret": key.Secret,
		"qr_code_url": key.URL,
		"backup_codes": key.BackupCodes,
	})
}

// VerifyTOTP verifies a TOTP code during setup
func (h *MFAHandler) VerifyTOTP(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	uid, err := uuid.Parse(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get the request body
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Get the secret from the session
	secret, err := c.Cookie("mfa_setup_secret")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "MFA setup not initiated"})
		return
	}

	// Validate the TOTP code
	if !utils.ValidateTOTPCode(secret, req.Code, h.mfaConfig) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid code"})
		return
	}

	// Get MFA settings
	settings, err := database.GetMFASettings(h.db, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get MFA settings"})
		return
	}

	// Create MFA device
	device := database.MFADevice{
		UserID:       uid,
		MFASettingsID: settings.ID,
		Name:         "Authenticator App",
		Method:       database.MFAMethodTOTP,
		Secret:       secret, // In a real implementation, you'd encrypt this
		Verified:     true,
		LastUsedAt:   nil,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := database.CreateMFADevice(h.db, &device); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create MFA device"})
		return
	}

	// Enable MFA for the user
	if err := database.EnableMFA(h.db, uid, database.MFAMethodTOTP); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enable MFA"})
		return
	}

	// Clear the setup cookie
	c.SetCookie("mfa_setup_secret", "", -1, "/", "", true, true)

	// Log the event
	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	h.auditLogger.LogEvent(c, utils.AuditEventMFAEnabled, utils.AuditSeverityInfo, 
		"MFA enabled", &uid, nil, ipAddress, userAgent, true, 
		map[string]interface{}{"method": "TOTP"})

	c.JSON(http.StatusOK, gin.H{
		"message": "MFA enabled successfully",
	})
}

// VerifyMFACode verifies an MFA code during login
func (h *MFAHandler) VerifyMFACode(c *gin.Context) {
	// Get the request body
	var req struct {
		UserID string `json:"user_id" binding:"required"`
		Code   string `json:"code" binding:"required"`
		Method string `json:"method" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	uid, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get user details
	var user database.User
	if err := h.db.First(&user, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Check if MFA is enabled
	if !user.TwoFactorEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "MFA not enabled for this user"})
		return
	}

	// Get MFA settings
	settings, err := database.GetMFASettings(h.db, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get MFA settings"})
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	// Handle different MFA methods
	switch database.MFAMethod(req.Method) {
	case database.MFAMethodTOTP:
		// Get the user's TOTP device
		var devices []database.MFADevice
		if err := h.db.Where("user_id = ? AND method = ? AND verified = ?", uid, database.MFAMethodTOTP, true).Find(&devices).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get MFA device"})
			return
		}

		if len(devices) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No TOTP device found"})
			return
		}

		// Validate the TOTP code
		valid := false
		for _, device := range devices {
			if utils.ValidateTOTPCode(device.Secret, req.Code, h.mfaConfig) {
				valid = true
				
				// Update last used time
				now := time.Now()
				device.LastUsedAt = &now
				device.UpdatedAt = now
				if err := database.UpdateMFADevice(h.db, &device); err != nil {
					// Log error but continue
					h.auditLogger.LogEvent(c, utils.AuditEventMFAFailed, utils.AuditSeverityError, 
						"Failed to update MFA device", &uid, nil, ipAddress, userAgent, false, 
						map[string]interface{}{"error": err.Error()})
				}
				break
			}
		}

		if !valid {
			// Log failed attempt
			h.auditLogger.LogEvent(c, utils.AuditEventMFAFailed, utils.AuditSeverityWarning, 
				"Invalid TOTP code", &uid, nil, ipAddress, userAgent, false, 
				map[string]interface{}{"method": "TOTP"})
			
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid code"})
			return
		}

	case database.MFAMethodBackup:
		// Check if the code matches any backup codes
		valid, err := h.validateBackupCode(uid, req.Code)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate backup code"})
			return
		}

		if !valid {
			// Log failed attempt
			h.auditLogger.LogEvent(c, utils.AuditEventMFAFailed, utils.AuditSeverityWarning, 
				"Invalid backup code", &uid, nil, ipAddress, userAgent, false, 
				map[string]interface{}{"method": "BACKUP"})
			
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid backup code"})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported MFA method"})
		return
	}

	// Update MFA settings with last verification time
	now := time.Now()
	settings.LastVerifiedAt = &now
	settings.UpdatedAt = now
	if err := h.db.Save(settings).Error; err != nil {
		// Log error but continue
		h.auditLogger.LogEvent(c, utils.AuditEventMFAFailed, utils.AuditSeverityError, 
			"Failed to update MFA settings", &uid, nil, ipAddress, userAgent, false, 
			map[string]interface{}{"error": err.Error()})
	}

	// Log successful verification
	h.auditLogger.LogEvent(c, utils.AuditEventLogin, utils.AuditSeverityInfo, 
		"MFA verification successful", &uid, nil, ipAddress, userAgent, true, 
		map[string]interface{}{"method": req.Method})

	c.JSON(http.StatusOK, gin.H{
		"message": "MFA verification successful",
	})
}

// DisableMFA disables MFA for a user
func (h *MFAHandler) DisableMFA(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	uid, err := uuid.Parse(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get the request body
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Get user details
	var user database.User
	if err := h.db.First(&user, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}

	// Disable MFA
	if err := database.DisableMFA(h.db, uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable MFA"})
		return
	}

	// Log the event
	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	h.auditLogger.LogEvent(c, utils.AuditEventMFADisabled, utils.AuditSeverityInfo, 
		"MFA disabled", &uid, nil, ipAddress, userAgent, true, nil)

	c.JSON(http.StatusOK, gin.H{
		"message": "MFA disabled successfully",
	})
}

// GetMFAStatus gets the MFA status for a user
func (h *MFAHandler) GetMFAStatus(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	uid, err := uuid.Parse(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get MFA settings
	settings, err := database.GetMFASettings(h.db, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get MFA settings"})
		return
	}

	// Get MFA devices
	devices, err := database.GetMFADevices(h.db, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get MFA devices"})
		return
	}

	// Get backup codes count
	backupCodes, err := database.GetUnusedBackupCodes(h.db, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get backup codes"})
		return
	}

	// Prepare device info
	var deviceInfo []gin.H
	for _, device := range devices {
		info := gin.H{
			"id":          device.ID,
			"name":        device.Name,
			"method":      device.Method,
			"verified":    device.Verified,
			"created_at":  device.CreatedAt,
			"last_used_at": device.LastUsedAt,
		}

		if device.PhoneNumber != nil {
			info["phone_number"] = *device.PhoneNumber
		}

		if device.Email != nil {
			info["email"] = *device.Email
		}

		deviceInfo = append(deviceInfo, info)
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled":        settings.Enabled,
		"default_method": settings.DefaultMethod,
		"devices":        deviceInfo,
		"backup_codes_remaining": len(backupCodes),
		"last_verified_at": settings.LastVerifiedAt,
	})
}

// GenerateBackupCodes generates new backup codes for a user
func (h *MFAHandler) GenerateBackupCodes(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	uid, err := uuid.Parse(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get the request body
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Get user details
	var user database.User
	if err := h.db.First(&user, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}

	// Get MFA settings
	settings, err := database.GetMFASettings(h.db, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get MFA settings"})
		return
	}

	// Generate new backup codes
	backupCodes, err := utils.GenerateBackupCodes(h.mfaConfig.BackupCodeCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate backup codes"})
		return
	}

	// Hash backup codes for storage
	var hashedBackupCodes []string
	for _, code := range backupCodes {
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash backup codes"})
			return
		}
		hashedBackupCodes = append(hashedBackupCodes, string(hash))
	}

	// Store backup codes in the database
	if err := database.CreateBackupCodes(h.db, uid, settings.ID, hashedBackupCodes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store backup codes"})
		return
	}

	// Log the event
	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	h.auditLogger.LogEvent(c, utils.AuditEventMFAEnabled, utils.AuditSeverityInfo, 
		"Backup codes regenerated", &uid, nil, ipAddress, userAgent, true, nil)

	c.JSON(http.StatusOK, gin.H{
		"backup_codes": backupCodes,
	})
}

// validateBackupCode validates a backup code and marks it as used if valid
func (h *MFAHandler) validateBackupCode(userID uuid.UUID, code string) (bool, error) {
	// Get unused backup codes
	codes, err := database.GetUnusedBackupCodes(h.db, userID)
	if err != nil {
		return false, err
	}

	// Check each code
	for _, backupCode := range codes {
		if err := bcrypt.CompareHashAndPassword([]byte(backupCode.Code), []byte(code)); err == nil {
			// Code matches, mark as used
			now := time.Now()
			backupCode.Used = true
			backupCode.UsedAt = &now
			
			if err := h.db.Save(&backupCode).Error; err != nil {
				return false, err
			}
			
			return true, nil
		}
	}
	
	return false, nil
}
