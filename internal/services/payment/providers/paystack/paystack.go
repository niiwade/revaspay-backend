package paystack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/models"
)

// PaystackProvider implements the payment.PaymentProvider interface for Paystack
type PaystackProvider struct {
	secretKey string
	publicKey string
	baseURL   string
}

// PaystackConfig holds configuration for the Paystack provider
type PaystackConfig struct {
	SecretKey string
	PublicKey string
	BaseURL   string
}

// NewPaystackProvider creates a new Paystack provider
func NewPaystackProvider(config PaystackConfig) *PaystackProvider {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.paystack.co"
	}

	return &PaystackProvider{
		secretKey: config.SecretKey,
		publicKey: config.PublicKey,
		baseURL:   baseURL,
	}
}

// InitiatePaymentRequest represents a request to initiate a payment
type InitiatePaymentRequest struct {
	Amount      int64  `json:"amount"`       // Amount in kobo (for NGN) or cents (for other currencies)
	Email       string `json:"email"`
	Currency    string `json:"currency"`
	Reference   string `json:"reference"`
	CallbackURL string `json:"callback_url"`
	Metadata    struct {
		CustomFields []struct {
			DisplayName string `json:"display_name"`
			VariableName string `json:"variable_name"`
			Value       string `json:"value"`
		} `json:"custom_fields"`
	} `json:"metadata"`
}

// InitiatePaymentResponse represents a response from Paystack
type InitiatePaymentResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		AuthorizationURL string `json:"authorization_url"`
		AccessCode       string `json:"access_code"`
		Reference        string `json:"reference"`
	} `json:"data"`
}

// VerifyPaymentResponse represents a response from Paystack verification
type VerifyPaymentResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Amount          int64  `json:"amount"`
		Currency        string `json:"currency"`
		TransactionDate string `json:"transaction_date"`
		Status          string `json:"status"`
		Reference       string `json:"reference"`
		Domain          string `json:"domain"`
		Metadata        struct {
			CustomFields []struct {
				DisplayName  string `json:"display_name"`
				VariableName string `json:"variable_name"`
				Value        string `json:"value"`
			} `json:"custom_fields"`
		} `json:"metadata"`
		GatewayResponse string `json:"gateway_response"`
		Channel         string `json:"channel"`
		IPAddress       string `json:"ip_address"`
		Log             struct {
			StartTime int64 `json:"start_time"`
			TimeSpent int   `json:"time_spent"`
			Attempts  int   `json:"attempts"`
			Errors    int   `json:"errors"`
			Success   bool  `json:"success"`
			Mobile    bool  `json:"mobile"`
			Input     []any `json:"input"`
			History   []struct {
				Type    string `json:"type"`
				Message string `json:"message"`
				Time    int    `json:"time"`
			} `json:"history"`
		} `json:"log"`
		Fees          int64  `json:"fees"`
		Authorization struct {
			AuthorizationCode string `json:"authorization_code"`
			CardType          string `json:"card_type"`
			Last4             string `json:"last4"`
			ExpMonth          string `json:"exp_month"`
			ExpYear           string `json:"exp_year"`
			Bin               string `json:"bin"`
			Bank              string `json:"bank"`
			Channel           string `json:"channel"`
			Signature         string `json:"signature"`
			Reusable          bool   `json:"reusable"`
			CountryCode       string `json:"country_code"`
			AccountName       string `json:"account_name"`
		} `json:"authorization"`
		Customer struct {
			ID           int    `json:"id"`
			FirstName    string `json:"first_name"`
			LastName     string `json:"last_name"`
			Email        string `json:"email"`
			CustomerCode string `json:"customer_code"`
			Phone        string `json:"phone"`
			Metadata     any    `json:"metadata"`
			RiskAction   string `json:"risk_action"`
		} `json:"customer"`
		Plan             any    `json:"plan"`
		RequestedAmount  int64  `json:"requested_amount"`
		PaidAt           string `json:"paid_at"`
		CreatedAt        string `json:"created_at"`
		ID               int    `json:"id"`
	} `json:"data"`
}

// WebhookPayload represents a Paystack webhook payload
type WebhookPayload struct {
	Event string `json:"event"`
	Data  struct {
		ID        int    `json:"id"`
		Domain    string `json:"domain"`
		Status    string `json:"status"`
		Reference string `json:"reference"`
		Amount    int64  `json:"amount"`
		Message   string `json:"message"`
		GatewayResponse string `json:"gateway_response"`
		PaidAt    string `json:"paid_at"`
		CreatedAt string `json:"created_at"`
		Channel   string `json:"channel"`
		Currency  string `json:"currency"`
		IPAddress string `json:"ip_address"`
		Metadata  struct {
			CustomFields []struct {
				DisplayName  string `json:"display_name"`
				VariableName string `json:"variable_name"`
				Value        string `json:"value"`
			} `json:"custom_fields"`
		} `json:"metadata"`
		Customer struct {
			ID           int    `json:"id"`
			FirstName    string `json:"first_name"`
			LastName     string `json:"last_name"`
			Email        string `json:"email"`
			CustomerCode string `json:"customer_code"`
			Phone        string `json:"phone"`
		} `json:"customer"`
		Authorization struct {
			AuthorizationCode string `json:"authorization_code"`
			CardType          string `json:"card_type"`
			Last4             string `json:"last4"`
			ExpMonth          string `json:"exp_month"`
			ExpYear           string `json:"exp_year"`
			Bin               string `json:"bin"`
			Bank              string `json:"bank"`
			Channel           string `json:"channel"`
			Signature         string `json:"signature"`
			Reusable          bool   `json:"reusable"`
			CountryCode       string `json:"country_code"`
		} `json:"authorization"`
		Fees int64 `json:"fees"`
	} `json:"data"`
}

// InitiatePayment initiates a payment with Paystack
func (p *PaystackProvider) InitiatePayment(payment *models.Payment) (string, error) {
	// Convert amount to the smallest currency unit (kobo for NGN, cents for USD, etc.)
	amount := int64(payment.Amount * 100)
	
	// Prepare request
	req := InitiatePaymentRequest{
		Amount:      amount,
		Email:       payment.CustomerEmail,
		Currency:    string(payment.Currency),
		Reference:   payment.Reference,
		CallbackURL: fmt.Sprintf("https://revaspay.com/payments/verify/%s", payment.Reference),
	}
	
	// Add metadata
	metadata := payment.Metadata
	if metadata != nil {
		customFields := []struct {
			DisplayName  string `json:"display_name"`
			VariableName string `json:"variable_name"`
			Value        string `json:"value"`
		}{}
		
		// Add payment ID
		customFields = append(customFields, struct {
			DisplayName  string `json:"display_name"`
			VariableName string `json:"variable_name"`
			Value        string `json:"value"`
		}{
			DisplayName:  "Payment ID",
			VariableName: "payment_id",
			Value:        payment.ID.String(),
		})
		
		// Add customer name if available
		if payment.CustomerName != "" {
			customFields = append(customFields, struct {
				DisplayName  string `json:"display_name"`
				VariableName string `json:"variable_name"`
				Value        string `json:"value"`
			}{
				DisplayName:  "Customer Name",
				VariableName: "customer_name",
				Value:        payment.CustomerName,
			})
		}
		
		req.Metadata.CustomFields = customFields
	}
	
	// Convert request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %w", err)
	}
	
	// Create HTTP request
	httpReq, err := http.NewRequest("POST", p.baseURL+"/transaction/initialize", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}
	
	// Add headers
	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")
	
	// Send request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}
	
	// Parse response
	var paystackResp InitiatePaymentResponse
	if err := json.Unmarshal(respBody, &paystackResp); err != nil {
		return "", fmt.Errorf("error parsing response: %w", err)
	}
	
	// Check if successful
	if !paystackResp.Status {
		return "", fmt.Errorf("paystack error: %s", paystackResp.Message)
	}
	
	// Update payment with provider reference
	payment.ProviderRef = paystackResp.Data.Reference
	
	// Return authorization URL
	return paystackResp.Data.AuthorizationURL, nil
}

// VerifyPayment verifies a payment with Paystack
func (p *PaystackProvider) VerifyPayment(reference string) (*models.Payment, error) {
	// Create HTTP request
	httpReq, err := http.NewRequest("GET", p.baseURL+"/transaction/verify/"+reference, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	
	// Add headers
	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")
	
	// Send request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}
	
	// Parse response
	var paystackResp VerifyPaymentResponse
	if err := json.Unmarshal(respBody, &paystackResp); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	
	// Check if successful
	if !paystackResp.Status {
		return nil, fmt.Errorf("paystack error: %s", paystackResp.Message)
	}
	
	// Create payment object
	payment := &models.Payment{
		ProviderRef: paystackResp.Data.Reference,
		Amount:      float64(paystackResp.Data.Amount) / 100, // Convert from kobo/cents to main unit
		Currency:    models.Currency(paystackResp.Data.Currency),
		ProviderFee: float64(paystackResp.Data.Fees) / 100,   // Convert from kobo/cents to main unit
	}
	
	// Set payment method
	payment.PaymentMethod = "card"
	if paystackResp.Data.Channel != "" {
		payment.PaymentMethod = paystackResp.Data.Channel
	}
	
	// Set payment details
	paymentDetails := map[string]interface{}{
		"authorization_code": paystackResp.Data.Authorization.AuthorizationCode,
		"card_type":          paystackResp.Data.Authorization.CardType,
		"last4":              paystackResp.Data.Authorization.Last4,
		"exp_month":          paystackResp.Data.Authorization.ExpMonth,
		"exp_year":           paystackResp.Data.Authorization.ExpYear,
		"bank":               paystackResp.Data.Authorization.Bank,
	}
	payment.PaymentDetails = models.JSON(paymentDetails)
	
	// Set status
	switch paystackResp.Data.Status {
	case "success":
		payment.Status = models.PaymentStatusCompleted
	case "failed":
		payment.Status = models.PaymentStatusFailed
	default:
		payment.Status = models.PaymentStatusPending
	}
	
	return payment, nil
}

// ProcessWebhook processes a webhook from Paystack
func (p *PaystackProvider) ProcessWebhook(data []byte) (*models.PaymentWebhook, error) {
	// Parse webhook payload
	var payload WebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("error parsing webhook payload: %w", err)
	}
	
	// Parse raw data into map for models.JSON
	var rawDataMap map[string]interface{}
	if err := json.Unmarshal(data, &rawDataMap); err != nil {
		return nil, fmt.Errorf("error parsing webhook raw data: %w", err)
	}

	// Create webhook object
	webhook := &models.PaymentWebhook{
		ID:        uuid.New(),
		Provider:  models.PaymentProviderPaystack,
		Event:     payload.Event,
		Reference: payload.Data.Reference,
		RawData:   models.JSON(rawDataMap),
		Processed: false,
	}
	
	return webhook, nil
}
