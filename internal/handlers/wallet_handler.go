package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/models"
	"github.com/revaspay/backend/internal/services/wallet"
	"gorm.io/gorm"
)

// WalletHandler handles wallet-related requests
type WalletHandler struct {
	db            *gorm.DB
	walletService *wallet.WalletService
}

// NewWalletHandler creates a new wallet handler
func NewWalletHandler(db *gorm.DB) *WalletHandler {
	return &WalletHandler{
		db:            db,
		walletService: wallet.NewWalletService(db),
	}
}

// GetWallets gets all wallets for the authenticated user
func (h *WalletHandler) GetWallets(c *gin.Context) {
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}
	
	wallets, err := h.walletService.GetWallets(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get wallets"})
		return
	}
	
	c.JSON(http.StatusOK, wallets)
}

// GetWallet gets a specific wallet by ID
func (h *WalletHandler) GetWallet(c *gin.Context) {
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	
	walletIDStr := c.Param("id")
	walletID, err := uuid.Parse(walletIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid wallet ID"})
		return
	}
	
	// Get the wallet
	wallet, err := h.walletService.GetWallet(walletID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
		return
	}
	
	// Verify wallet belongs to user
	userID, _ := uuid.Parse(userIDStr)
	if wallet.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}
	
	c.JSON(http.StatusOK, wallet)
}

// CreateWallet creates a new wallet for the authenticated user
func (h *WalletHandler) CreateWallet(c *gin.Context) {
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	
	var input struct {
		Currency models.Currency `json:"currency" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}
	
	// Check if wallet already exists
	var existingWallet models.Wallet
	result := h.db.Where("user_id = ? AND currency = ?", userID, input.Currency).First(&existingWallet)
	if result.Error == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "wallet already exists for this currency", "wallet": existingWallet})
		return
	}
	
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check existing wallet"})
		return
	}
	
	// Create new wallet
	wallet, err := h.walletService.GetOrCreateWallet(userID, input.Currency)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create wallet"})
		return
	}
	
	c.JSON(http.StatusCreated, wallet)
}

// GetTransactionHistory gets transaction history for a wallet
func (h *WalletHandler) GetTransactionHistory(c *gin.Context) {
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	
	walletIDStr := c.Param("id")
	walletID, err := uuid.Parse(walletIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid wallet ID"})
		return
	}
	
	// Verify wallet belongs to user
	var wallet models.Wallet
	if err := h.db.First(&wallet, "id = ?", walletID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
		return
	}
	
	userID, _ := uuid.Parse(userIDStr)
	if wallet.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}
	
	// Get pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	
	transactions, total, err := h.walletService.GetTransactionHistory(walletID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get transaction history"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"pagination": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetAutoWithdrawConfig gets auto-withdraw configuration for the authenticated user
func (h *WalletHandler) GetAutoWithdrawConfig(c *gin.Context) {
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}
	
	config, err := h.walletService.GetAutoWithdrawConfig(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get auto-withdraw config"})
		return
	}
	
	if config == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false})
		return
	}
	
	c.JSON(http.StatusOK, config)
}

// UpdateAutoWithdrawConfig updates auto-withdraw configuration for the authenticated user
func (h *WalletHandler) UpdateAutoWithdrawConfig(c *gin.Context) {
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}
	
	var input struct {
		Enabled        bool           `json:"enabled"`
		Threshold      float64        `json:"threshold"`
		Currency       models.Currency `json:"currency"`
		WithdrawMethod string         `json:"withdraw_method"`
		DestinationID  uuid.UUID      `json:"destination_id"`
	}
	
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	config, err := h.walletService.UpdateAutoWithdrawConfig(
		userID,
		input.Enabled,
		input.Threshold,
		input.Currency,
		input.WithdrawMethod,
		input.DestinationID,
	)
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update auto-withdraw config"})
		return
	}
	
	c.JSON(http.StatusOK, config)
}
