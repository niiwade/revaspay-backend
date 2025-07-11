package momo

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const (
	// API endpoints
	sandboxBaseURL = "https://sandbox.momodeveloper.mtn.com"
	prodBaseURL    = "https://momodeveloper.mtn.com"

	// Collection API endpoints
	tokenEndpoint         = "/collection/token/"
	requestToPayEndpoint  = "/collection/v1_0/requesttopay"
	transactionEndpoint   = "/collection/v1_0/requesttopay/%s"
	accountBalanceEndpoint = "/collection/v1_0/account/balance"
	
	// Disbursement API endpoints
	disbursementTokenEndpoint = "/disbursement/token/"
	transferEndpoint          = "/disbursement/v1_0/transfer"
	transferStatusEndpoint    = "/disbursement/v1_0/transfer/%s"
	disbursementBalanceEndpoint = "/disbursement/v1_0/account/balance"
)

// MoMoClient represents the MTN Mobile Money API client
type MoMoClient struct {
	BaseURL          string
	SubscriptionKey  string
	CollectionAPIUser string
	CollectionAPIKey  string
	DisbursementAPIUser string
	DisbursementAPIKey  string
	HTTPClient       *http.Client
	UseSandbox       bool
}

// NewMoMoClient creates a new MTN Mobile Money API client
func NewMoMoClient(subscriptionKey, collectionAPIUser, collectionAPIKey, 
                   disbursementAPIUser, disbursementAPIKey string, useSandbox bool) *MoMoClient {
	baseURL := prodBaseURL
	if useSandbox {
		baseURL = sandboxBaseURL
	}

	return &MoMoClient{
		BaseURL:          baseURL,
		SubscriptionKey:  subscriptionKey,
		CollectionAPIUser: collectionAPIUser,
		CollectionAPIKey:  collectionAPIKey,
		DisbursementAPIUser: disbursementAPIUser,
		DisbursementAPIKey:  disbursementAPIKey,
		HTTPClient:       &http.Client{Timeout: 30 * time.Second},
		UseSandbox:       useSandbox,
	}
}

// TokenResponse represents the OAuth token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// GetCollectionToken gets an OAuth token for the Collection API
func (c *MoMoClient) GetCollectionToken() (*TokenResponse, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(c.CollectionAPIUser + ":" + c.CollectionAPIKey))
	
	req, err := http.NewRequest("POST", c.BaseURL+tokenEndpoint, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Ocp-Apim-Subscription-Key", c.SubscriptionKey)
	
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get token: %s, status: %d", string(body), resp.StatusCode)
	}
	
	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}
	
	return &tokenResp, nil
}

// GetDisbursementToken gets an OAuth token for the Disbursement API
func (c *MoMoClient) GetDisbursementToken() (*TokenResponse, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(c.DisbursementAPIUser + ":" + c.DisbursementAPIKey))
	
	req, err := http.NewRequest("POST", c.BaseURL+disbursementTokenEndpoint, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Ocp-Apim-Subscription-Key", c.SubscriptionKey)
	
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get disbursement token: %s, status: %d", string(body), resp.StatusCode)
	}
	
	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}
	
	return &tokenResp, nil
}

// RequestToPayRequest represents the request to pay payload
type RequestToPayRequest struct {
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	ExternalID   string `json:"externalId"`
	PayerMessage string `json:"payerMessage"`
	PayeeNote    string `json:"payeeNote"`
	Payer        Payer  `json:"payer"`
}

// Payer represents the payer information
type Payer struct {
	PartyIDType string `json:"partyIdType"`
	PartyID     string `json:"partyId"`
}

// RequestToPayResponse represents the response from a request to pay
type RequestToPayResponse struct {
	ReferenceID string `json:"referenceId"`
}

// RequestToPay initiates a payment request to a mobile money user
func (c *MoMoClient) RequestToPay(request RequestToPayRequest) (string, error) {
	token, err := c.GetCollectionToken()
	if err != nil {
		return "", err
	}
	
	reqBody, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	
	referenceID := uuid.New().String()
	req, err := http.NewRequest("POST", c.BaseURL+requestToPayEndpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("X-Reference-Id", referenceID)
	req.Header.Set("X-Target-Environment", c.getEnvironment())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Ocp-Apim-Subscription-Key", c.SubscriptionKey)
	
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusAccepted {
		body, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("request to pay failed: %s, status: %d", string(body), resp.StatusCode)
	}
	
	return referenceID, nil
}

// TransactionStatus represents the status of a transaction
type TransactionStatus struct {
	Amount        string    `json:"amount"`
	Currency      string    `json:"currency"`
	ExternalID    string    `json:"externalId"`
	Payer         Payer     `json:"payer"`
	PayerMessage  string    `json:"payerMessage"`
	PayeeNote     string    `json:"payeeNote"`
	Status        string    `json:"status"`
	Reason        string    `json:"reason,omitempty"`
	FinancialTransactionID string `json:"financialTransactionId,omitempty"`
}

// GetTransactionStatus gets the status of a transaction
func (c *MoMoClient) GetTransactionStatus(referenceID string) (*TransactionStatus, error) {
	token, err := c.GetCollectionToken()
	if err != nil {
		return nil, err
	}
	
	endpoint := fmt.Sprintf(transactionEndpoint, referenceID)
	req, err := http.NewRequest("GET", c.BaseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("X-Target-Environment", c.getEnvironment())
	req.Header.Set("Ocp-Apim-Subscription-Key", c.SubscriptionKey)
	
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get transaction status: %s, status: %d", string(body), resp.StatusCode)
	}
	
	var status TransactionStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	
	return &status, nil
}

// Balance represents the account balance
type Balance struct {
	AvailableBalance string `json:"availableBalance"`
	Currency         string `json:"currency"`
}

// GetAccountBalance gets the account balance
func (c *MoMoClient) GetAccountBalance() (*Balance, error) {
	token, err := c.GetCollectionToken()
	if err != nil {
		return nil, err
	}
	
	req, err := http.NewRequest("GET", c.BaseURL+accountBalanceEndpoint, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("X-Target-Environment", c.getEnvironment())
	req.Header.Set("Ocp-Apim-Subscription-Key", c.SubscriptionKey)
	
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get account balance: %s, status: %d", string(body), resp.StatusCode)
	}
	
	var balance Balance
	if err := json.NewDecoder(resp.Body).Decode(&balance); err != nil {
		return nil, err
	}
	
	return &balance, nil
}

// TransferRequest represents a transfer request
type TransferRequest struct {
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	ExternalID   string `json:"externalId"`
	PayerMessage string `json:"payerMessage"`
	PayeeNote    string `json:"payeeNote"`
	Payee        Payer  `json:"payee"` // Reusing Payer struct as the structure is the same
}

// Transfer sends money to a mobile money user
func (c *MoMoClient) Transfer(request TransferRequest) (string, error) {
	token, err := c.GetDisbursementToken()
	if err != nil {
		return "", err
	}
	
	reqBody, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	
	referenceID := uuid.New().String()
	req, err := http.NewRequest("POST", c.BaseURL+transferEndpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("X-Reference-Id", referenceID)
	req.Header.Set("X-Target-Environment", c.getEnvironment())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Ocp-Apim-Subscription-Key", c.SubscriptionKey)
	
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusAccepted {
		body, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("transfer failed: %s, status: %d", string(body), resp.StatusCode)
	}
	
	return referenceID, nil
}

// GetTransferStatus gets the status of a transfer
func (c *MoMoClient) GetTransferStatus(referenceID string) (*TransactionStatus, error) {
	token, err := c.GetDisbursementToken()
	if err != nil {
		return nil, err
	}
	
	endpoint := fmt.Sprintf(transferStatusEndpoint, referenceID)
	req, err := http.NewRequest("GET", c.BaseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("X-Target-Environment", c.getEnvironment())
	req.Header.Set("Ocp-Apim-Subscription-Key", c.SubscriptionKey)
	
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get transfer status: %s, status: %d", string(body), resp.StatusCode)
	}
	
	var status TransactionStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	
	return &status, nil
}

// GetDisbursementBalance gets the disbursement account balance
func (c *MoMoClient) GetDisbursementBalance() (*Balance, error) {
	token, err := c.GetDisbursementToken()
	if err != nil {
		return nil, err
	}
	
	req, err := http.NewRequest("GET", c.BaseURL+disbursementBalanceEndpoint, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("X-Target-Environment", c.getEnvironment())
	req.Header.Set("Ocp-Apim-Subscription-Key", c.SubscriptionKey)
	
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get disbursement balance: %s, status: %d", string(body), resp.StatusCode)
	}
	
	var balance Balance
	if err := json.NewDecoder(resp.Body).Decode(&balance); err != nil {
		return nil, err
	}
	
	return &balance, nil
}

// Helper method to get the environment string
func (c *MoMoClient) getEnvironment() string {
	if c.UseSandbox {
		return "sandbox"
	}
	return "production"
}
