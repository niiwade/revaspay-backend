package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/services/banking"
	"gorm.io/gorm"
)

// BankingHandler handles bank account related requests
type BankingHandler struct {
	db              *gorm.DB
	bankingService  *banking.GhanaBankingService
}

// NewBankingHandler creates a new banking handler
func NewBankingHandler(db *gorm.DB) *BankingHandler {
	return &BankingHandler{
		db:              db,
		bankingService:  banking.NewGhanaBankingService(db),
	}
}

// LinkBankAccount links a bank account to a user's account
func (h *BankingHandler) LinkBankAccount(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse request
	var req struct {
		AccountNumber string `json:"account_number" binding:"required"`
		AccountName   string `json:"account_name" binding:"required"`
		BankName      string `json:"bank_name" binding:"required"`
		BankCode      string `json:"bank_code" binding:"required"`
		BranchCode    string `json:"branch_code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert to service struct
	bankDetails := banking.BankAccountDetails{
		AccountNumber: req.AccountNumber,
		AccountName:   req.AccountName,
		BankName:      req.BankName,
		BankCode:      req.BankCode,
		BranchCode:    req.BranchCode,
	}

	// Link bank account
	account, err := h.bankingService.LinkBankAccount(userID.(uuid.UUID), bankDetails)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Bank account linked successfully",
		"data":    account,
	})
}

// GetBankAccounts retrieves all bank accounts for a user
func (h *BankingHandler) GetBankAccounts(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get bank accounts
	accounts, err := h.bankingService.GetBankAccounts(userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   accounts,
	})
}

// GetBankAccount retrieves a specific bank account
func (h *BankingHandler) GetBankAccount(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get account ID from URL
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	// Get bank account
	var account database.BankAccount
	if err := h.db.Where("id = ? AND user_id = ?", accountID, userID).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bank account not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   account,
	})
}

// UpdateBankAccount updates a bank account
func (h *BankingHandler) UpdateBankAccount(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get account ID from URL
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	// Check if account exists and belongs to user
	var account database.BankAccount
	if err := h.db.Where("id = ? AND user_id = ?", accountID, userID).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bank account not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Parse request
	var req struct {
		AccountName string `json:"account_name"`
		IsActive    *bool  `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update fields
	updates := map[string]interface{}{}
	
	if req.AccountName != "" {
		updates["account_name"] = req.AccountName
	}
	
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	// Apply updates
	if err := h.db.Model(&account).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get updated account
	if err := h.db.First(&account, accountID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Bank account updated successfully",
		"data":    account,
	})
}

// DeleteBankAccount deletes a bank account
func (h *BankingHandler) DeleteBankAccount(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get account ID from URL
	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	// Check if account exists and belongs to user
	var account database.BankAccount
	if err := h.db.Where("id = ? AND user_id = ?", accountID, userID).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bank account not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Soft delete the account
	if err := h.db.Delete(&account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Bank account deleted successfully",
	})
}

// GetBanks retrieves a list of supported Ghanaian banks
func (h *BankingHandler) GetBanks(c *gin.Context) {
	// In production, this would come from a database or API
	// For now, we'll return a static list of major Ghanaian banks
	banks := []map[string]string{
		{"name": "Ghana Commercial Bank", "code": "GCB"},
		{"name": "Ecobank Ghana", "code": "ECO"},
		{"name": "Fidelity Bank Ghana", "code": "FBG"},
		{"name": "Zenith Bank Ghana", "code": "ZBG"},
		{"name": "Standard Chartered Bank Ghana", "code": "SCB"},
		{"name": "Absa Bank Ghana", "code": "ABSA"},
		{"name": "Consolidated Bank Ghana", "code": "CBG"},
		{"name": "Agricultural Development Bank", "code": "ADB"},
		{"name": "National Investment Bank", "code": "NIB"},
		{"name": "Prudential Bank", "code": "PBL"},
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   banks,
	})
}

// VerifyBankAccount verifies a bank account with the bank
func (h *BankingHandler) VerifyBankAccount(c *gin.Context) {
	// Parse request
	var req struct {
		AccountNumber string `json:"account_number" binding:"required"`
		BankCode      string `json:"bank_code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// In production, this would call the actual bank's API
	// For now, we'll simulate a successful verification
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Bank account verified successfully",
		"data": map[string]string{
			"account_number": req.AccountNumber,
			"account_name":   "John Doe", // This would come from the bank's API
			"bank_code":      req.BankCode,
			"bank_name":      "Ghana Commercial Bank", // This would come from the bank's API
		},
	})
}
