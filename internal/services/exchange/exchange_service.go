package exchange

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ExchangeRateService provides real-time currency exchange rates
type ExchangeRateService struct {
	baseURL      string
	cacheTimeout time.Duration
	rateCache    map[string]CachedRate
	mutex        sync.RWMutex
}

// CachedRate represents a cached exchange rate with expiration
type CachedRate struct {
	Rate      float64
	Timestamp time.Time
}

// ExchangeRateResponse represents the response from the ExchangeRate-API
type ExchangeRateResponse struct {
	Result   string             `json:"result"`
	Base     string             `json:"base"`
	Updated  string             `json:"time_last_update_utc"`
	Rates    map[string]float64 `json:"rates"`
	Provider string             `json:"provider"`
}

// NewExchangeRateService creates a new exchange rate service
// Uses the free ExchangeRate-API which doesn't require an API key
func NewExchangeRateService() *ExchangeRateService {
	return &ExchangeRateService{
		baseURL:      "https://open.er-api.com/v6/latest",
		cacheTimeout: 1 * time.Hour,
		rateCache:    make(map[string]CachedRate),
	}
}

// GetExchangeRate gets the exchange rate between two currencies
func (s *ExchangeRateService) GetExchangeRate(fromCurrency, toCurrency string) (float64, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s-%s", fromCurrency, toCurrency)
	s.mutex.RLock()
	cachedRate, exists := s.rateCache[cacheKey]
	s.mutex.RUnlock()

	if exists && time.Since(cachedRate.Timestamp) < s.cacheTimeout {
		return cachedRate.Rate, nil
	}

	// Make API request to the free ExchangeRate-API
	url := fmt.Sprintf("%s/%s", s.baseURL, fromCurrency)

	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch exchange rate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("exchange rate API returned status code %d", resp.StatusCode)
	}

	var rateResp ExchangeRateResponse
	if err := json.NewDecoder(resp.Body).Decode(&rateResp); err != nil {
		return 0, fmt.Errorf("failed to decode exchange rate response: %w", err)
	}

	if rateResp.Result != "success" {
		return 0, fmt.Errorf("exchange rate API returned unsuccessful response")
	}

	rate, exists := rateResp.Rates[toCurrency]
	if !exists {
		return 0, fmt.Errorf("exchange rate not found for currency %s", toCurrency)
	}

	// Update cache
	s.mutex.Lock()
	s.rateCache[cacheKey] = CachedRate{
		Rate:      rate,
		Timestamp: time.Now(),
	}
	s.mutex.Unlock()

	return rate, nil
}

// ConvertAmount converts an amount from one currency to another
func (s *ExchangeRateService) ConvertAmount(amount float64, fromCurrency, toCurrency string) (float64, error) {
	if fromCurrency == toCurrency {
		return amount, nil
	}

	rate, err := s.GetExchangeRate(fromCurrency, toCurrency)
	if err != nil {
		return 0, err
	}

	return amount * rate, nil
}

// GetAllRates gets exchange rates for a base currency against all supported currencies
func (s *ExchangeRateService) GetAllRates(baseCurrency string) (map[string]float64, error) {
	url := fmt.Sprintf("%s/%s", s.baseURL, baseCurrency)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch exchange rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange rate API returned status code %d", resp.StatusCode)
	}

	var rateResp ExchangeRateResponse
	if err := json.NewDecoder(resp.Body).Decode(&rateResp); err != nil {
		return nil, fmt.Errorf("failed to decode exchange rate response: %w", err)
	}

	if rateResp.Result != "success" {
		return nil, fmt.Errorf("exchange rate API returned unsuccessful response")
	}

	return rateResp.Rates, nil
}
