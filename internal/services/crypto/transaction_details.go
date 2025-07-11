package crypto

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum"
)

// TransactionDetails represents details of a blockchain transaction
type TransactionDetails struct {
	Hash        string
	BlockNumber uint64
	BlockHash   string
	GasUsed     uint64
	Success     bool
}

// GetTransactionDetails retrieves detailed information about a transaction
func (s *BaseService) GetTransactionDetails(txHash string) (*TransactionDetails, error) {
	hash := common.HexToHash(txHash)
	
	// Get transaction
	_, isPending, err := s.client.TransactionByHash(context.Background(), hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	
	// If transaction is still pending, return with pending status
	if isPending {
		return &TransactionDetails{
			Hash:    txHash,
			Success: false, // Not confirmed yet
		}, nil
	}
	
	// Get transaction receipt to check status
	receipt, err := s.client.TransactionReceipt(context.Background(), hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction receipt: %w", err)
	}
	
	// In Ethereum, status 1 means success, 0 means failure
	success := receipt.Status == 1
	
	// Get block information - we only need to verify the block exists
	_, err = s.client.BlockByHash(context.Background(), receipt.BlockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}
	
	return &TransactionDetails{
		Hash:        txHash,
		BlockNumber: receipt.BlockNumber.Uint64(),
		BlockHash:   receipt.BlockHash.Hex(),
		GasUsed:     receipt.GasUsed,
		Success:     success,
	}, nil
}

// EstimateGasForTransaction estimates the gas required for a transaction
func (s *BaseService) EstimateGasForTransaction(fromAddress, toAddress string, amount *big.Int, data []byte) (uint64, error) {
	from := common.HexToAddress(fromAddress)
	to := common.HexToAddress(toAddress)
	
	gasLimit, err := s.client.EstimateGas(context.Background(), 
		ethereum.CallMsg{
			From:  from,
			To:    &to,
			Value: amount,
			Data:  data,
		})
	
	if err != nil {
		return 0, fmt.Errorf("failed to estimate gas: %w", err)
	}
	
	return gasLimit, nil
}
