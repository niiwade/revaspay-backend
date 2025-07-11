package queue

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// JobQueue defines the interface for job queue operations
type JobQueue interface {
	EnqueueJob(jobType JobType, payload interface{}) (string, error)
	GetJob(id string) (*Job, error)
	UpdateJobStatus(id string, status JobStatus, result interface{}, err error) error
}

// PaymentJobHandler handles payment-related jobs
type PaymentJobHandler struct {
	db            *gorm.DB
	cryptoService *MockCryptoService
	queue         JobQueue
}

// ProcessPayment processes an international payment job
func (h *PaymentJobHandler) ProcessPayment(job Job) error {
	var payload ProcessPaymentPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal process payment payload: %w", err)
	}

	// Implementation for test
	return nil
}

// NotifyPaymentStatus notifies about payment status changes
func (h *PaymentJobHandler) NotifyPaymentStatus(job Job) error {
	// Implementation for test
	return nil
}

// Note: ProcessPaymentPayload is imported from payment_jobs.go

// MockCryptoService is a mock implementation of the crypto service
type MockCryptoService struct {
	mock.Mock
}

func (m *MockCryptoService) SendTransaction(fromWallet *database.Wallet, toAddress string, amount string) (string, error) {
	args := m.Called(fromWallet, toAddress, amount)
	return args.String(0), args.Error(1)
}

func (m *MockCryptoService) GetBalance(address string) (string, error) {
	args := m.Called(address)
	return args.String(0), args.Error(1)
}

func (m *MockCryptoService) ValidateAddress(address string) bool {
	args := m.Called(address)
	return args.Bool(0)
}

// MockQueue is a mock implementation of the job queue
type MockQueue struct {
	mock.Mock
}

func (m *MockQueue) EnqueueJob(jobType JobType, payload interface{}) (string, error) {
	args := m.Called(jobType, payload)
	return args.String(0), args.Error(1)
}

func (m *MockQueue) GetJob(id string) (*Job, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Job), args.Error(1)
}

func (m *MockQueue) UpdateJobStatus(id string, status JobStatus, result interface{}, err error) error {
	args := m.Called(id, status, result, err)
	return args.Error(0)
}

func setupTestDBWithModels(t *testing.T) *gorm.DB {
	// In a real implementation, we would use an in-memory SQLite database
	// but for this test we'll use a mock DB setup
	db, err := gorm.Open(nil, &gorm.Config{})
	require.NoError(t, err)

	// Migrate the necessary schemas
	err = db.AutoMigrate(
		&database.Wallet{},
		&database.InternationalPayment{},
		&database.CryptoTransaction{},
		&Job{},
	)
	require.NoError(t, err)

	return db
}

func TestProcessPaymentHandler(t *testing.T) {
	db := setupTestDBWithModels(t)
	mockCryptoService := new(MockCryptoService)
	mockQueue := new(MockQueue)

	// Create a test wallet
	wallet := database.Wallet{
		ID:                uuid.New(),
		UserID:            uuid.New(),
		Balance:           1000.0,
		Currency:          "USD",
		AutoWithdraw:      false,
		WithdrawThreshold: 500.0,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	err := db.Create(&wallet).Error
	require.NoError(t, err)

	// Create a test payment
	payment := database.InternationalPayment{
		ID:                uuid.New(),
		UserID:            wallet.UserID,
		BankTransactionID: uuid.New(),
		CryptoTxID:        uuid.New(),
		VendorName:        "Test Vendor",
		VendorAddress:     "0xabcdef1234567890",
		AmountCedis:       100.0,
		AmountCrypto:      "100.0",
		ExchangeRate:      1.0,
		Status:            "pending",
		Description:       "Test payment",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	err = db.Create(&payment).Error
	require.NoError(t, err)

	// Create payload
	payload := ProcessPaymentPayload{
		PaymentID:        payment.ID,
		UserID:           wallet.UserID,
		BankAccountID:    uuid.New(),
		WalletID:         wallet.ID,
		VendorName:       payment.VendorName,
		RecipientAddress: payment.VendorAddress,
		Amount:           payment.AmountCedis,
		Currency:         "USD",
		Description:      payment.Description,
		Reference:        "test-reference",
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	// Create job
	job := Job{
		ID:        uuid.New(),
		Type:      JobTypeProcessPayment,
		Payload:   payloadBytes,
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Set up expectations
	mockCryptoService.On("ValidateAddress", payment.VendorAddress).Return(true)
	mockCryptoService.On("SendTransaction", mock.AnythingOfType("*database.Wallet"), payment.VendorAddress, payment.AmountCrypto).
		Return("0xtx123456789", nil)
	mockQueue.On("UpdateJobStatus", job.ID.String(), JobStatusCompleted, nil, nil).Return(nil)

	// Create handler
	handler := &PaymentJobHandler{
		db:            db,
		cryptoService: mockCryptoService,
		queue:         mockQueue,
	}

	// Execute handler
	err = handler.ProcessPayment(job)
	assert.NoError(t, err)

	// Verify transaction was created
	var tx database.CryptoTransaction
	err = db.Where("payment_id = ?", payment.ID).First(&tx).Error
	assert.NoError(t, err)
	assert.Equal(t, "0xtx123456789", tx.TransactionHash)
	assert.Equal(t, "pending", tx.Status)

	// Verify payment was updated
	var updatedPayment database.InternationalPayment
	err = db.First(&updatedPayment, payment.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, "processing", updatedPayment.Status)

	// Verify mock expectations
	mockCryptoService.AssertExpectations(t)
	mockQueue.AssertExpectations(t)
}

func TestProcessPaymentHandler_InvalidAddress(t *testing.T) {
	db := setupTestDBWithModels(t)
	mockCryptoService := new(MockCryptoService)
	mockQueue := new(MockQueue)

	// Create a test wallet
	wallet := database.Wallet{
		ID:                uuid.New(),
		UserID:            uuid.New(),
		Balance:           1000.0,
		Currency:          "USD",
		AutoWithdraw:      false,
		WithdrawThreshold: 500.0,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	err := db.Create(&wallet).Error
	require.NoError(t, err)

	// Create a test payment
	payment := database.InternationalPayment{
		ID:                uuid.New(),
		UserID:            wallet.UserID,
		BankTransactionID: uuid.New(),
		CryptoTxID:        uuid.New(),
		VendorName:        "Test Vendor",
		VendorAddress:     "invalid-address",
		AmountCedis:       100.0,
		AmountCrypto:      "100.0",
		ExchangeRate:      1.0,
		Status:            "pending",
		Description:       "Test payment",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	err = db.Create(&payment).Error
	require.NoError(t, err)

	// Create payload
	payload := ProcessPaymentPayload{
		PaymentID:        payment.ID,
		UserID:           wallet.UserID,
		BankAccountID:    uuid.New(),
		WalletID:         wallet.ID,
		VendorName:       payment.VendorName,
		RecipientAddress: payment.VendorAddress,
		Amount:           payment.AmountCedis,
		Currency:         "USD",
		Description:      payment.Description,
		Reference:        "test-reference",
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	// Create job
	job := Job{
		ID:        uuid.New(),
		Type:      JobTypeProcessPayment,
		Payload:   payloadBytes,
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Set up expectations
	mockCryptoService.On("ValidateAddress", payment.VendorAddress).Return(false)
	mockQueue.On("UpdateJobStatus", job.ID.String(), JobStatusFailed, nil, mock.AnythingOfType("*errors.errorString")).Return(nil)

	// Create handler
	handler := &PaymentJobHandler{
		db:            db,
		cryptoService: mockCryptoService,
		queue:         mockQueue,
	}

	// Execute handler
	err = handler.ProcessPayment(job)
	assert.NoError(t, err)

	// Verify payment was updated to failed
	var updatedPayment database.InternationalPayment
	err = db.First(&updatedPayment, payment.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, "failed", updatedPayment.Status)

	// Verify mock expectations
	mockCryptoService.AssertExpectations(t)
	mockQueue.AssertExpectations(t)
}

func TestNotifyPaymentStatusHandler(t *testing.T) {
	db := setupTestDBWithModels(t)
	mockQueue := new(MockQueue)

	// Create a test payment
	payment := database.InternationalPayment{
		ID:                uuid.New(),
		UserID:            uuid.New(),
		BankTransactionID: uuid.New(),
		CryptoTxID:        uuid.New(),
		VendorName:        "Test Vendor",
		VendorAddress:     "0xabcdef1234567890",
		AmountCedis:       100.0,
		AmountCrypto:      "100.0",
		ExchangeRate:      1.0,
		Status:            "completed",
		Description:       "Test payment",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	err := db.Create(&payment).Error
	require.NoError(t, err)

	// Create payload
	type NotifyPaymentStatusPayload struct {
		PaymentID uuid.UUID `json:"payment_id"`
		Status    string    `json:"status"`
	}

	payload := NotifyPaymentStatusPayload{
		PaymentID: payment.ID,
		Status:    "completed",
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	// Create job
	job := Job{
		ID:        uuid.New(),
		Type:      JobTypeNotifyPaymentStatus,
		Payload:   payloadBytes,
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Set up expectations
	mockQueue.On("UpdateJobStatus", job.ID.String(), JobStatusCompleted, nil, nil).Return(nil)

	// Create handler
	handler := &PaymentJobHandler{
		db:    db,
		queue: mockQueue,
	}

	// Execute handler
	err = handler.NotifyPaymentStatus(job)
	assert.NoError(t, err)

	// Verify mock expectations
	mockQueue.AssertExpectations(t)

	// In a real implementation, we would verify that notifications were sent
	// This would require mocking notification services
}
