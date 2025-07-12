package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/models"
	"github.com/revaspay/backend/internal/services/wallet"
	"gorm.io/gorm"
)

// AdminWalletHandler handles admin wallet-related requests
type AdminWalletHandler struct {
	db            *gorm.DB
	walletService *wallet.WalletService
}

// NewAdminWalletHandler creates a new admin wallet handler
func NewAdminWalletHandler(db *gorm.DB) *AdminWalletHandler {
	return &AdminWalletHandler{
		db:            db,
		walletService: wallet.NewWalletService(db),
	}
}

// GetAllWallets gets all wallets in the system with pagination
func (h *AdminWalletHandler) GetAllWallets(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	
	var wallets []models.Wallet
	var total int64
	
	// Count total wallets
	if err := h.db.Model(&models.Wallet{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count wallets"})
		return
	}
	
	// Get paginated wallets with user information
	offset := (page - 1) * pageSize
	if err := h.db.Preload("User").Offset(offset).Limit(pageSize).Find(&wallets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get wallets"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"wallets": wallets,
		"pagination": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetUserWallets gets all wallets for a specific user
func (h *AdminWalletHandler) GetUserWallets(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}
	
	// Check if user exists
	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	
	// Get user's wallets
	wallets, err := h.walletService.GetWallets(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get wallets"})
		return
	}
	
	c.JSON(http.StatusOK, wallets)
}

// GetWalletTransactions gets all transactions for a specific wallet
func (h *AdminWalletHandler) GetWalletTransactions(c *gin.Context) {
	walletIDStr := c.Param("id")
	walletID, err := uuid.Parse(walletIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid wallet ID"})
		return
	}
	
	// Check if wallet exists
	var wallet models.Wallet
	if err := h.db.First(&wallet, "id = ?", walletID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
		return
	}
	
	// Get pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	
	// Get transactions
	transactions, total, err := h.walletService.GetTransactionHistory(walletID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get transactions"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"wallet":       wallet,
		"pagination": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// AdjustWalletBalance manually adjusts a wallet balance (admin only)
func (h *AdminWalletHandler) AdjustWalletBalance(c *gin.Context) {
	walletIDStr := c.Param("id")
	walletID, err := uuid.Parse(walletIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid wallet ID"})
		return
	}
	
	// Check if wallet exists
	var wallet models.Wallet
	if err := h.db.First(&wallet, "id = ?", walletID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
		return
	}
	
	var input struct {
		Amount      float64                `json:"amount" binding:"required"`
		Type        string                 `json:"type" binding:"required"` // credit or debit
		Reference   string                 `json:"reference" binding:"required"`
		Description string                 `json:"description" binding:"required"`
		Metadata    map[string]interface{} `json:"metadata"`
	}
	
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Validate input
	if input.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount must be positive"})
		return
	}
	
	if input.Type != "credit" && input.Type != "debit" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be 'credit' or 'debit'"})
		return
	}
	
	// Perform the adjustment
	var transaction *models.Transaction
	var adjustErr error
	
	if input.Type == "credit" {
		transaction, adjustErr = h.walletService.Credit(
			walletID,
			input.Amount,
			"admin_adjustment",
			input.Reference,
			input.Description,
			input.Metadata,
		)
	} else {
		transaction, adjustErr = h.walletService.Debit(
			walletID,
			input.Amount,
			"admin_adjustment",
			input.Reference,
			input.Description,
			input.Metadata,
		)
	}
	
	if adjustErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": adjustErr.Error()})
		return
	}
	
	// Get updated wallet
	if err := h.db.First(&wallet, "id = ?", walletID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get updated wallet"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"wallet":      wallet,
		"transaction": transaction,
		"message":     "Wallet balance adjusted successfully",
	})
}

// GetAllAutoWithdrawConfigs gets all auto-withdraw configurations
func (h *AdminWalletHandler) GetAllAutoWithdrawConfigs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	
	var configs []models.AutoWithdrawConfig
	var total int64
	
	// Count total configs
	if err := h.db.Model(&models.AutoWithdrawConfig{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count auto-withdraw configs"})
		return
	}
	
	// Get paginated configs with user information
	offset := (page - 1) * pageSize
	if err := h.db.Preload("User").Offset(offset).Limit(pageSize).Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get auto-withdraw configs"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"configs": configs,
		"pagination": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}
