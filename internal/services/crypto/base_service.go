package crypto

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"gorm.io/gorm"
)

// BaseService handles interactions with the Base blockchain
type BaseService struct {
	client     *ethclient.Client
	db         *gorm.DB
	networkURL string
	chainID    int64
}

// NewBaseService creates a new Base blockchain service
func NewBaseService(db *gorm.DB) *BaseService {
	// Base Mainnet RPC URL - in production, use environment variables
	baseRPC := "https://mainnet.base.org"
	// Base Chain ID is 8453
	baseChainID := int64(8453)

	client, err := ethclient.Dial(baseRPC)
	if err != nil {
		// In production, handle this error properly
		panic(fmt.Sprintf("Failed to connect to Base blockchain: %v", err))
	}

	return &BaseService{
		client:     client,
		db:         db,
		networkURL: baseRPC,
		chainID:    baseChainID,
	}
}

// CreateBaseWallet generates a new wallet on Base for a user
func (s *BaseService) CreateBaseWallet(userID uuid.UUID) (*database.CryptoWallet, error) {
	// Generate wallet using standard Ethereum methods since Base is EVM-compatible
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	// Get Ethereum address
	publicKeyECDSA := privateKey.PublicKey
	address := crypto.PubkeyToAddress(publicKeyECDSA).Hex()

	// Encrypt private key with user-specific passphrase
	// In production, use proper key management systems like AWS KMS or HashiCorp Vault
	encryptedKey := encryptPrivateKey(privateKey, userID.String())

	wallet := &database.CryptoWallet{
		UserID:       userID,
		Address:      address,
		EncryptedKey: encryptedKey,
		WalletType:   "BASE",
		Network:      "base_mainnet",
		IsActive:     true,
	}

	if err := s.db.Create(wallet).Error; err != nil {
		return nil, err
	}

	return wallet, nil
}

// GetBalance retrieves the balance for a wallet address
func (s *BaseService) GetBalance(address string) (*big.Int, error) {
	account := common.HexToAddress(address)
	balance, err := s.client.BalanceAt(context.Background(), account, nil)
	if err != nil {
		return nil, err
	}
	return balance, nil
}

// SendTransaction sends a transaction on the Base blockchain
func (s *BaseService) SendTransaction(fromWallet *database.CryptoWallet, toAddress string, amount *big.Int) (string, error) {
	// Decrypt private key
	privateKey, err := decryptPrivateKey(fromWallet.EncryptedKey, fromWallet.UserID.String())
	if err != nil {
		return "", err
	}

	// Get nonce for the account
	fromAddress := common.HexToAddress(fromWallet.Address)
	nonce, err := s.client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return "", err
	}

	// Get gas price
	gasPrice, err := s.client.SuggestGasPrice(context.Background())
	if err != nil {
		return "", err
	}

	// Create transaction
	to := common.HexToAddress(toAddress)
	gasLimit := uint64(21000) // Standard gas limit for ETH transfers

	tx := types.NewTransaction(nonce, to, amount, gasLimit, gasPrice, nil)

	// Sign transaction
	chainID := big.NewInt(s.chainID)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		return "", err
	}

	// Send transaction
	err = s.client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return "", err
	}

	// Return transaction hash
	return signedTx.Hash().Hex(), nil
}

// GetTransaction retrieves transaction details
func (s *BaseService) GetTransaction(txHash string) (*types.Transaction, bool, error) {
	hash := common.HexToHash(txHash)
	tx, isPending, err := s.client.TransactionByHash(context.Background(), hash)
	if err != nil {
		return nil, false, err
	}
	return tx, isPending, nil
}

// Helper functions for key management
// In production, use a proper key management system

// encryptPrivateKey encrypts a private key with a user-specific passphrase
func encryptPrivateKey(privateKey *ecdsa.PrivateKey, passphrase string) string {
	// This is a simplified implementation
	// In production, use proper encryption like AES-GCM with the passphrase
	// For now, we're just appending a simple marker using the passphrase
	privateKeyBytes := crypto.FromECDSA(privateKey)
	hexKey := hex.EncodeToString(privateKeyBytes)
	
	// Use first character of passphrase as a simple marker
	// This is NOT secure and just for demonstration
	if len(passphrase) > 0 {
		return hexKey + "_" + string(passphrase[0])
	}
	return hexKey
}

// decryptPrivateKey decrypts a private key with a user-specific passphrase
func decryptPrivateKey(encryptedKey string, passphrase string) (*ecdsa.PrivateKey, error) {
	// This is a simplified implementation
	// In production, use proper decryption like AES-GCM with the passphrase
	
	// Remove the simple marker if it exists
	hexKey := encryptedKey
	if len(passphrase) > 0 && len(encryptedKey) > 2 && encryptedKey[len(encryptedKey)-2] == '_' {
		// Check if the marker matches the first character of the passphrase
		if encryptedKey[len(encryptedKey)-1] != passphrase[0] {
			return nil, fmt.Errorf("invalid passphrase")
		}
		// Remove marker
		hexKey = encryptedKey[:len(encryptedKey)-2]
	}
	
	privateKeyBytes, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, err
	}
	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}
