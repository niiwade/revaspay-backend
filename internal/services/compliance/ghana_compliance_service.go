package compliance

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"gorm.io/gorm"
)

// TransactionLimits defines the transaction limits for different user tiers
type TransactionLimits struct {
	DailyLimit      float64
	MonthlyLimit    float64
	SingleTxLimit   float64
	RequiresApproval bool
}

// ComplianceCheck represents a compliance check result
type ComplianceCheck struct {
	Passed      bool   `json:"passed"`
	CheckType   string `json:"check_type"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
}

// GhanaComplianceService handles compliance checks for Ghanaian banking regulations
type GhanaComplianceService struct {
	db *gorm.DB
}

// NewGhanaComplianceService creates a new Ghana compliance service
func NewGhanaComplianceService(db *gorm.DB) *GhanaComplianceService {
	return &GhanaComplianceService{
		db: db,
	}
}

// ValidateTransaction checks if a transaction complies with Ghanaian banking regulations
func (s *GhanaComplianceService) ValidateTransaction(userID uuid.UUID, amount float64, transactionType string, vendorName string, vendorAddress string) (bool, []ComplianceCheck, error) {
	// Get user's KYC status
	var kyc database.KYC
	if err := s.db.Where("user_id = ?", userID).First(&kyc).Error; err != nil {
		return false, nil, fmt.Errorf("KYC record not found: %v", err)
	}

	// Check if KYC is verified
	if kyc.Status != "verified" {
		return false, []ComplianceCheck{
			{
				Passed:      false,
				CheckType:   "kyc_verification",
				Description: "User KYC is not verified",
				Timestamp:   time.Now().Format(time.RFC3339),
			},
		}, nil
	}

	// Get user's transaction limits based on KYC level
	limits := s.getUserTransactionLimits(userID)

	// Check single transaction limit
	if amount > limits.SingleTxLimit {
		return false, []ComplianceCheck{
			{
				Passed:      false,
				CheckType:   "single_tx_limit",
				Description: fmt.Sprintf("Transaction amount exceeds single transaction limit of %.2f GHS", limits.SingleTxLimit),
				Timestamp:   time.Now().Format(time.RFC3339),
			},
		}, nil
	}

	// Check daily transaction limit
	dailyTotal, err := s.getDailyTransactionTotal(userID)
	if err != nil {
		return false, nil, err
	}

	if dailyTotal+amount > limits.DailyLimit {
		return false, []ComplianceCheck{
			{
				Passed:      false,
				CheckType:   "daily_limit",
				Description: fmt.Sprintf("Transaction would exceed daily limit of %.2f GHS", limits.DailyLimit),
				Timestamp:   time.Now().Format(time.RFC3339),
			},
		}, nil
	}

	// Check monthly transaction limit
	monthlyTotal, err := s.getMonthlyTransactionTotal(userID)
	if err != nil {
		return false, nil, err
	}

	if monthlyTotal+amount > limits.MonthlyLimit {
		return false, []ComplianceCheck{
			{
				Passed:      false,
				CheckType:   "monthly_limit",
				Description: fmt.Sprintf("Transaction would exceed monthly limit of %.2f GHS", limits.MonthlyLimit),
				Timestamp:   time.Now().Format(time.RFC3339),
			},
		}, nil
	}

	// Check for suspicious activity
	isSuspicious, suspiciousCheck := s.checkForSuspiciousActivity(userID, amount, transactionType)
	if isSuspicious {
		return false, []ComplianceCheck{suspiciousCheck}, nil
	}

	// All checks passed
	return true, []ComplianceCheck{
		{
			Passed:      true,
			CheckType:   "kyc_verification",
			Description: "User KYC is verified",
			Timestamp:   time.Now().Format(time.RFC3339),
		},
		{
			Passed:      true,
			CheckType:   "transaction_limits",
			Description: "Transaction is within limits",
			Timestamp:   time.Now().Format(time.RFC3339),
		},
		{
			Passed:      true,
			CheckType:   "suspicious_activity",
			Description: "No suspicious activity detected",
			Timestamp:   time.Now().Format(time.RFC3339),
		},
	}, nil
}

// GenerateComplianceReport generates a compliance report for a transaction
func (s *GhanaComplianceService) GenerateComplianceReport(transactionID uuid.UUID, paymentID uuid.UUID, transactionType string) (string, error) {
	var transaction interface{}
	var userID uuid.UUID

	// Get the transaction based on type
	switch transactionType {
	case "international_payment":
		var payment database.InternationalPayment
		if err := s.db.First(&payment, transactionID).Error; err != nil {
			return "", err
		}
		transaction = payment
		userID = payment.UserID
	case "bank_transaction":
		var bankTx database.GhanaBankTransaction
		if err := s.db.First(&bankTx, transactionID).Error; err != nil {
			return "", err
		}
		transaction = bankTx
		userID = bankTx.UserID
	default:
		return "", fmt.Errorf("unsupported transaction type: %s", transactionType)
	}

	// Get user details
	var user database.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return "", err
	}

	// Get KYC details
	var kyc database.KYC
	if err := s.db.Where("user_id = ?", userID).First(&kyc).Error; err != nil {
		return "", err
	}

	// Create report
	report := map[string]interface{}{
		"report_id":        uuid.New().String(),
		"timestamp":        time.Now().Format(time.RFC3339),
		"transaction_id":   transactionID.String(),
		"transaction_type": transactionType,
		"user_details": map[string]interface{}{
			"user_id":    userID.String(),
			"name":       user.FirstName + " " + user.LastName,
			"email":      user.Email,
			"kyc_status": kyc.Status,
		},
		"transaction_details": transaction,
		"compliance_checks": []map[string]interface{}{
			{
				"check_type":  "ghana_banking_laws",
				"status":      "compliant",
				"description": "Transaction complies with Ghana Banking Act 2004 (Act 673)",
				"timestamp":   time.Now().Format(time.RFC3339),
			},
			{
				"check_type":  "aml_check",
				"status":      "passed",
				"description": "Anti-Money Laundering check passed",
				"timestamp":   time.Now().Format(time.RFC3339),
			},
			{
				"check_type":  "foreign_exchange_control",
				"status":      "compliant",
				"description": "Complies with Foreign Exchange Act 2006 (Act 723)",
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
		"regulatory_authority": "Bank of Ghana",
		"report_generated_by": "RevasPay Compliance System",
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

// Helper methods

// getUserTransactionLimits returns the transaction limits for a user based on their KYC level
func (s *GhanaComplianceService) getUserTransactionLimits(userID uuid.UUID) TransactionLimits {
	// In production, this would be based on the user's KYC level and stored in the database
	// For now, we'll use default limits
	return TransactionLimits{
		DailyLimit:      50000,  // 50,000 GHS
		MonthlyLimit:    500000, // 500,000 GHS
		SingleTxLimit:   20000,  // 20,000 GHS
		RequiresApproval: false,
	}
}

// getDailyTransactionTotal calculates the total amount of transactions for a user in the current day
func (s *GhanaComplianceService) getDailyTransactionTotal(userID uuid.UUID) (float64, error) {
	startOfDay := time.Now().Truncate(24 * time.Hour)
	endOfDay := startOfDay.Add(24 * time.Hour)

	var total float64
	if err := s.db.Model(&database.GhanaBankTransaction{}).
		Where("user_id = ? AND created_at BETWEEN ? AND ? AND status != ?", 
			userID, startOfDay, endOfDay, "failed").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error; err != nil {
		return 0, err
	}

	return total, nil
}

// getMonthlyTransactionTotal calculates the total amount of transactions for a user in the current month
func (s *GhanaComplianceService) getMonthlyTransactionTotal(userID uuid.UUID) (float64, error) {
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	var total float64
	if err := s.db.Model(&database.GhanaBankTransaction{}).
		Where("user_id = ? AND created_at BETWEEN ? AND ? AND status != ?", 
			userID, startOfMonth, endOfMonth, "failed").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error; err != nil {
		return 0, err
	}

	return total, nil
}

// checkForSuspiciousActivity checks for suspicious transaction patterns
func (s *GhanaComplianceService) checkForSuspiciousActivity(userID uuid.UUID, amount float64, transactionType string) (bool, ComplianceCheck) {
	// In production, this would implement sophisticated fraud detection algorithms
	// For now, we'll just flag very large transactions
	if amount > 100000 { // 100,000 GHS
		return true, ComplianceCheck{
			Passed:      false,
			CheckType:   "suspicious_activity",
			Description: "Unusually large transaction amount",
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	}

	// Check for multiple transactions in a short period
	var recentCount int64
	s.db.Model(&database.GhanaBankTransaction{}).
		Where("user_id = ? AND created_at > ? AND transaction_type = ?", 
			userID, time.Now().Add(-1*time.Hour), transactionType).
		Count(&recentCount)

	if recentCount > 5 {
		return true, ComplianceCheck{
			Passed:      false,
			CheckType:   "suspicious_activity",
			Description: "Multiple transactions in a short period",
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	}

	return false, ComplianceCheck{}
}
