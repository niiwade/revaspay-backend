package payment

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/revaspay/backend/internal/models"
	"github.com/revaspay/backend/internal/services/wallet"
	"gorm.io/gorm"
)

// PaymentService handles payment operations
type PaymentService struct {
	db            *gorm.DB
	walletService *wallet.WalletService
	providers     map[models.PaymentProvider]PaymentProvider
}

// PaymentProvider interface for different payment providers
type PaymentProvider interface {
	InitiatePayment(payment *models.Payment) (string, error)
	VerifyPayment(reference string) (*models.Payment, error)
	ProcessWebhook(webhookData []byte) (*models.PaymentWebhook, error)
}

// NewPaymentService creates a new payment service
func NewPaymentService(db *gorm.DB, walletService *wallet.WalletService) *PaymentService {
	service := &PaymentService{
		db:            db,
		walletService: walletService,
		providers:     make(map[models.PaymentProvider]PaymentProvider),
	}
	
	// Register providers here when they're implemented
	// service.RegisterProvider(models.PaymentProviderPaystack, paystack.NewPaystackProvider(config))
	// service.RegisterProvider(models.PaymentProviderStripe, stripe.NewStripeProvider(config))
	// service.RegisterProvider(models.PaymentProviderPayPal, paypal.NewPayPalProvider(config))
	// service.RegisterProvider(models.PaymentProviderCrypto, crypto.NewCryptoProvider(config))
	
	return service
}

// RegisterProvider registers a payment provider
func (s *PaymentService) RegisterProvider(name models.PaymentProvider, provider PaymentProvider) {
	s.providers[name] = provider
}

// CreatePaymentLink creates a new payment link
func (s *PaymentService) CreatePaymentLink(userID uuid.UUID, title, description string, amount float64, currency models.Currency, metadata map[string]interface{}) (*models.PaymentLink, error) {
	// Generate a unique slug
	baseSlug := slug.Make(title)
	uniqueSlug := fmt.Sprintf("%s-%s", baseSlug, uuid.New().String()[:8])
	
	paymentLink := models.PaymentLink{
		UserID:      userID,
		Title:       title,
		Description: description,
		Amount:      amount,
		Currency:    currency,
		Slug:        uniqueSlug,
		Active:      true,
		Metadata:    models.JSON(metadata),
	}
	
	if err := s.db.Create(&paymentLink).Error; err != nil {
		return nil, fmt.Errorf("error creating payment link: %w", err)
	}
	
	return &paymentLink, nil
}

// GetPaymentLink gets a payment link by ID
func (s *PaymentService) GetPaymentLink(id uuid.UUID) (*models.PaymentLink, error) {
	var paymentLink models.PaymentLink
	if err := s.db.First(&paymentLink, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("error finding payment link: %w", err)
	}
	return &paymentLink, nil
}

// GetPaymentLinkBySlug gets a payment link by slug
func (s *PaymentService) GetPaymentLinkBySlug(slug string) (*models.PaymentLink, error) {
	var paymentLink models.PaymentLink
	if err := s.db.First(&paymentLink, "slug = ? AND active = true", slug).Error; err != nil {
		return nil, fmt.Errorf("error finding payment link: %w", err)
	}
	return &paymentLink, nil
}

// GetUserPaymentLinks gets all payment links for a user
func (s *PaymentService) GetUserPaymentLinks(userID uuid.UUID) ([]models.PaymentLink, error) {
	var links []models.PaymentLink
	if err := s.db.Where("user_id = ?", userID).Find(&links).Error; err != nil {
		return nil, fmt.Errorf("error finding payment links: %w", err)
	}
	return links, nil
}

// UpdatePaymentLink updates a payment link
func (s *PaymentService) UpdatePaymentLink(id uuid.UUID, userID uuid.UUID, updates map[string]interface{}) (*models.PaymentLink, error) {
	var paymentLink models.PaymentLink
	if err := s.db.First(&paymentLink, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		return nil, fmt.Errorf("error finding payment link: %w", err)
	}
	
	if err := s.db.Model(&paymentLink).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("error updating payment link: %w", err)
	}
	
	return &paymentLink, nil
}

// DeletePaymentLink deletes a payment link
func (s *PaymentService) DeletePaymentLink(id uuid.UUID, userID uuid.UUID) error {
	result := s.db.Delete(&models.PaymentLink{}, "id = ? AND user_id = ?", id, userID)
	if result.Error != nil {
		return fmt.Errorf("error deleting payment link: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("payment link not found or not owned by user")
	}
	return nil
}

// InitiatePayment initiates a payment using the specified provider
func (s *PaymentService) InitiatePayment(userID uuid.UUID, provider models.PaymentProvider, amount float64, currency models.Currency, customerEmail, customerName string, metadata map[string]interface{}) (*models.Payment, string, error) {
	// Check if provider is supported
	paymentProvider, ok := s.providers[provider]
	if !ok {
		return nil, "", fmt.Errorf("unsupported payment provider: %s", provider)
	}
	
	// Generate a unique reference
	reference := fmt.Sprintf("REV-%s", uuid.New().String()[:12])
	
	// Create payment record
	payment := models.Payment{
		UserID:        userID,
		Amount:        amount,
		Currency:      currency,
		Provider:      provider,
		Status:        models.PaymentStatusPending,
		Reference:     reference,
		CustomerEmail: customerEmail,
		CustomerName:  customerName,
		Metadata:      models.JSON(metadata),
	}
	
	if err := s.db.Create(&payment).Error; err != nil {
		return nil, "", fmt.Errorf("error creating payment record: %w", err)
	}
	
	// Initiate payment with provider
	checkoutURL, err := paymentProvider.InitiatePayment(&payment)
	if err != nil {
		// Update payment status to failed
		s.db.Model(&payment).Updates(map[string]interface{}{
			"status": models.PaymentStatusFailed,
			"error":  err.Error(),
		})
		return nil, "", fmt.Errorf("error initiating payment: %w", err)
	}
	
	return &payment, checkoutURL, nil
}

// InitiatePaymentFromLink initiates a payment from a payment link
func (s *PaymentService) InitiatePaymentFromLink(paymentLinkID uuid.UUID, provider models.PaymentProvider, customerEmail, customerName string) (*models.Payment, string, error) {
	// Get payment link
	var paymentLink models.PaymentLink
	if err := s.db.First(&paymentLink, "id = ? AND active = true", paymentLinkID).Error; err != nil {
		return nil, "", fmt.Errorf("error finding payment link: %w", err)
	}
	
	// Create metadata with payment link info
	metadata := map[string]interface{}{
		"payment_link_id":   paymentLink.ID.String(),
		"payment_link_slug": paymentLink.Slug,
		"payment_link_title": paymentLink.Title,
	}
	
	// Add original metadata if any
	if paymentLink.Metadata != nil {
		originalMetadata := map[string]interface{}(paymentLink.Metadata)
		for k, v := range originalMetadata {
			metadata[k] = v
		}
	}
	
	// Initiate payment
	return s.InitiatePayment(
		paymentLink.UserID,
		provider,
		paymentLink.Amount,
		paymentLink.Currency,
		customerEmail,
		customerName,
		metadata,
	)
}

// VerifyPayment verifies a payment using the specified provider
func (s *PaymentService) VerifyPayment(reference string) (*models.Payment, error) {
	// Find payment by reference
	var payment models.Payment
	if err := s.db.First(&payment, "reference = ?", reference).Error; err != nil {
		return nil, fmt.Errorf("error finding payment: %w", err)
	}
	
	// Get provider
	provider, ok := s.providers[payment.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported payment provider: %s", payment.Provider)
	}
	
	// Verify payment with provider
	updatedPayment, err := provider.VerifyPayment(reference)
	if err != nil {
		return nil, fmt.Errorf("error verifying payment: %w", err)
	}
	
	// Update payment record
	if err := s.db.Model(&payment).Updates(map[string]interface{}{
		"status":        updatedPayment.Status,
		"provider_ref":  updatedPayment.ProviderRef,
		"provider_fee":  updatedPayment.ProviderFee,
		"payment_method": updatedPayment.PaymentMethod,
		"payment_details": updatedPayment.PaymentDetails,
		"receipt_url":   updatedPayment.ReceiptURL,
	}).Error; err != nil {
		return nil, fmt.Errorf("error updating payment record: %w", err)
	}
	
	// If payment is completed, credit user's wallet
	if updatedPayment.Status == models.PaymentStatusCompleted {
		if err := s.processSuccessfulPayment(&payment); err != nil {
			return nil, fmt.Errorf("error processing successful payment: %w", err)
		}
	}
	
	return &payment, nil
}

// ProcessWebhook processes a webhook from a payment provider
func (s *PaymentService) ProcessWebhook(provider models.PaymentProvider, data []byte) (*models.PaymentWebhook, error) {
	// Get provider
	paymentProvider, ok := s.providers[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported payment provider: %s", provider)
	}
	
	// Process webhook with provider
	webhook, err := paymentProvider.ProcessWebhook(data)
	if err != nil {
		return nil, fmt.Errorf("error processing webhook: %w", err)
	}
	
	// Save webhook
	if err := s.db.Create(webhook).Error; err != nil {
		return nil, fmt.Errorf("error saving webhook: %w", err)
	}
	
	// If webhook has a payment reference, update the payment
	if webhook.Reference != "" {
		var payment models.Payment
		if err := s.db.First(&payment, "reference = ?", webhook.Reference).Error; err == nil {
			// Update payment with webhook data
			payment.WebhookReceived = true
			payment.WebhookData = webhook.RawData
			
			// If webhook indicates payment is completed, update status
			if strings.Contains(strings.ToLower(webhook.Event), "success") || 
			   strings.Contains(strings.ToLower(webhook.Event), "complete") {
				payment.Status = models.PaymentStatusCompleted
				
				// Process successful payment
				if err := s.processSuccessfulPayment(&payment); err != nil {
					return nil, fmt.Errorf("error processing successful payment: %w", err)
				}
			}
			
			// Save payment
			s.db.Save(&payment)
			
			// Update webhook with payment ID
			webhook.PaymentID = &payment.ID
			s.db.Save(webhook)
		}
	}
	
	return webhook, nil
}

// processSuccessfulPayment handles a successful payment by crediting the user's wallet
func (s *PaymentService) processSuccessfulPayment(payment *models.Payment) error {
	// Get or create wallet for user
	wallet, err := s.walletService.GetOrCreateWallet(payment.UserID, payment.Currency)
	if err != nil {
		return fmt.Errorf("error getting wallet: %w", err)
	}
	
	// Calculate net amount (after fees)
	netAmount := payment.Amount - payment.Fee - payment.ProviderFee
	
	// Credit wallet
	metadata := map[string]interface{}{
		"payment_id":      payment.ID.String(),
		"payment_reference": payment.Reference,
		"provider":        string(payment.Provider),
		"provider_ref":    payment.ProviderRef,
	}
	
	_, err = s.walletService.Credit(
		wallet.ID,
		netAmount,
		"payment",
		payment.Reference,
		fmt.Sprintf("Payment from %s", payment.CustomerEmail),
		metadata,
	)
	
	if err != nil {
		return fmt.Errorf("error crediting wallet: %w", err)
	}
	
	// Mark payment as processed
	payment.Status = models.PaymentStatusCompleted
	s.db.Save(payment)
	
	return nil
}

// GetPayment gets a payment by ID
func (s *PaymentService) GetPayment(id uuid.UUID) (*models.Payment, error) {
	var payment models.Payment
	if err := s.db.First(&payment, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("error finding payment: %w", err)
	}
	return &payment, nil
}

// GetPaymentByReference gets a payment by reference
func (s *PaymentService) GetPaymentByReference(reference string) (*models.Payment, error) {
	var payment models.Payment
	if err := s.db.First(&payment, "reference = ?", reference).Error; err != nil {
		return nil, fmt.Errorf("error finding payment: %w", err)
	}
	return &payment, nil
}

// GetUserPayments gets all payments for a user
func (s *PaymentService) GetUserPayments(userID uuid.UUID, page, pageSize int) ([]models.Payment, int64, error) {
	var payments []models.Payment
	var total int64
	
	// Count total records
	if err := s.db.Model(&models.Payment{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("error counting payments: %w", err)
	}
	
	// Get paginated records
	offset := (page - 1) * pageSize
	if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&payments).Error; err != nil {
		return nil, 0, fmt.Errorf("error finding payments: %w", err)
	}
	
	return payments, total, nil
}

// InitiateCryptoPayment initiates a cryptocurrency payment
func (s *PaymentService) InitiateCryptoPayment(userID uuid.UUID, amount float64, currency models.Currency, network, cryptoCurrency string, metadata map[string]interface{}) (*models.Payment, *models.CryptoPayment, error) {
	// Generate a unique reference
	reference := fmt.Sprintf("CRYPTO-%s", uuid.New().String()[:12])
	
	// Create payment record
	payment := models.Payment{
		UserID:        userID,
		Amount:        amount,
		Currency:      currency,
		Provider:      models.PaymentProviderCrypto,
		Status:        models.PaymentStatusPending,
		Reference:     reference,
		PaymentMethod: "crypto",
		Metadata:      models.JSON(metadata),
	}
	
	// Get crypto provider
	provider, ok := s.providers[models.PaymentProviderCrypto]
	if !ok {
		return nil, nil, errors.New("crypto payment provider not configured")
	}
	
	// Begin transaction
	tx := s.db.Begin()
	
	// Save payment
	if err := tx.Create(&payment).Error; err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("error creating payment record: %w", err)
	}
	
	// Generate crypto address (this would be implemented in the crypto provider)
	cryptoPaymentData := &models.CryptoPayment{
		PaymentID: payment.ID,
		Network:   network,
		Currency:  cryptoCurrency,
		Status:    models.PaymentStatusPending,
	}
	
	// Call provider to get address
	_, err := provider.InitiatePayment(&payment)
	if err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("error initiating crypto payment: %w", err)
	}
	
	// Extract address from provider response
	paymentDetails := map[string]interface{}{}
	paymentDetailsBytes, _ := json.Marshal(payment.PaymentDetails)
	if err := json.Unmarshal(paymentDetailsBytes, &paymentDetails); err == nil {
		if address, ok := paymentDetails["address"].(string); ok {
			cryptoPaymentData.Address = address
		}
		if amount, ok := paymentDetails["crypto_amount"].(string); ok {
			cryptoPaymentData.Amount = amount
		}
	}
	
	// Save crypto payment data
	if err := tx.Create(cryptoPaymentData).Error; err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("error creating crypto payment record: %w", err)
	}
	
	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, nil, fmt.Errorf("error committing transaction: %w", err)
	}
	
	return &payment, cryptoPaymentData, nil
}

// UpdateCryptoPayment updates a cryptocurrency payment
func (s *PaymentService) UpdateCryptoPayment(cryptoPaymentID uuid.UUID, txHash string, confirmations int, status models.PaymentStatus) error {
	// Find crypto payment
	var cryptoPayment models.CryptoPayment
	if err := s.db.First(&cryptoPayment, "id = ?", cryptoPaymentID).Error; err != nil {
		return fmt.Errorf("error finding crypto payment: %w", err)
	}
	
	// Update crypto payment
	cryptoPayment.TxHash = txHash
	cryptoPayment.Confirmations = confirmations
	cryptoPayment.Status = status
	
	if err := s.db.Save(&cryptoPayment).Error; err != nil {
		return fmt.Errorf("error updating crypto payment: %w", err)
	}
	
	// If payment is completed, update main payment and process
	if status == models.PaymentStatusCompleted {
		var payment models.Payment
		if err := s.db.First(&payment, "id = ?", cryptoPayment.PaymentID).Error; err != nil {
			return fmt.Errorf("error finding payment: %w", err)
		}
		
		payment.Status = models.PaymentStatusCompleted
		payment.PaymentDetails = models.JSON(map[string]interface{}{
			"tx_hash":      txHash,
			"confirmations": confirmations,
			"network":      cryptoPayment.Network,
			"currency":     cryptoPayment.Currency,
			"address":      cryptoPayment.Address,
			"amount":       cryptoPayment.Amount,
		})
		
		if err := s.db.Save(&payment).Error; err != nil {
			return fmt.Errorf("error updating payment: %w", err)
		}
		
		// Process successful payment
		if err := s.processSuccessfulPayment(&payment); err != nil {
			return fmt.Errorf("error processing successful payment: %w", err)
		}
	}
	
	return nil
}
