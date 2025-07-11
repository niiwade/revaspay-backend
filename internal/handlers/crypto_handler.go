package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/services/crypto"
	"gorm.io/gorm"
)

// CryptoHandler handles cryptocurrency wallet related requests
type CryptoHandler struct {
	db          *gorm.DB
	baseService *crypto.BaseService
}

// NewCryptoHandler creates a new crypto handler
func NewCryptoHandler(db *gorm.DB) *CryptoHandler {
	return &CryptoHandler{
		db:          db,
		baseService: crypto.NewBaseService(db),
	}
}

// CreateWallet creates a new Base blockchain wallet for a user
func (h *CryptoHandler) CreateWallet(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Check if user already has a Base wallet
	var existingWallet database.CryptoWallet
	result := h.db.Where("user_id = ? AND wallet_type = ?", userID, "BASE").First(&existingWallet)
	if result.RowsAffected > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User already has a Base wallet"})
		return
	}

	// Create wallet
	wallet, err := h.baseService.CreateBaseWallet(userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Base wallet created successfully",
		"data": map[string]interface{}{
			"wallet_id": wallet.ID,
			"address":   wallet.Address,
			"type":      wallet.WalletType,
			"network":   wallet.Network,
		},
	})
}

// GetWallets retrieves all crypto wallets for a user
func (h *CryptoHandler) GetWallets(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get wallets
	var wallets []database.CryptoWallet
	if err := h.db.Where("user_id = ?", userID).Find(&wallets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Remove sensitive data
	var safeWallets []map[string]interface{}
	for _, wallet := range wallets {
		safeWallets = append(safeWallets, map[string]interface{}{
			"id":         wallet.ID,
			"address":    wallet.Address,
			"wallet_type": wallet.WalletType,
			"network":    wallet.Network,
			"is_active":  wallet.IsActive,
			"created_at": wallet.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   safeWallets,
	})
}

// GetWallet retrieves a specific wallet
func (h *CryptoHandler) GetWallet(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get wallet ID from URL
	walletID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid wallet ID"})
		return
	}

	// Get wallet
	var wallet database.CryptoWallet
	if err := h.db.Where("id = ? AND user_id = ?", walletID, userID).First(&wallet).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Get wallet balance
	balance, err := h.baseService.GetBalance(wallet.Address)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get wallet balance"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": map[string]interface{}{
			"id":         wallet.ID,
			"address":    wallet.Address,
			"wallet_type": wallet.WalletType,
			"network":    wallet.Network,
			"is_active":  wallet.IsActive,
			"balance":    balance.String(),
			"created_at": wallet.CreatedAt,
		},
	})
}

// GetTransactions retrieves all transactions for a wallet
func (h *CryptoHandler) GetTransactions(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get wallet ID from URL
	walletID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid wallet ID"})
		return
	}

	// Check if wallet belongs to user
	var wallet database.CryptoWallet
	if err := h.db.Where("id = ? AND user_id = ?", walletID, userID).First(&wallet).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Get transactions
	var transactions []database.CryptoTransaction
	if err := h.db.Where("wallet_id = ?", walletID).Order("created_at DESC").Find(&transactions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   transactions,
	})
}

// GetTransaction retrieves a specific transaction
func (h *CryptoHandler) GetTransaction(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get transaction ID from URL
	txID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid transaction ID"})
		return
	}

	// Get transaction
	var transaction database.CryptoTransaction
	if err := h.db.Where("id = ? AND user_id = ?", txID, userID).First(&transaction).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// If transaction has a hash, get on-chain details
	var onChainDetails interface{} = nil
	if transaction.TransactionHash != "" {
		tx, isPending, err := h.baseService.GetTransaction(transaction.TransactionHash)
		if err == nil {
			onChainDetails = map[string]interface{}{
				"hash":       tx.Hash().Hex(),
				"is_pending": isPending,
				"gas_price":  tx.GasPrice().String(),
				"gas_limit":  tx.Gas(),
				"nonce":      tx.Nonce(),
				"value":      tx.Value().String(),
				"to":         tx.To().Hex(),
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": map[string]interface{}{
			"transaction":      transaction,
			"onchain_details": onChainDetails,
		},
	})
}
