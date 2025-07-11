package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"

	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/utils"
)

// MockDB is a mock implementation of the database
type MockDB struct {
	mock.Mock
}

// NewMockDB creates a new mock database for testing
func NewMockDB() *gorm.DB {
	// We're returning a real gorm.DB instance for the handler
	// The actual mocking is done in the test functions
	return &gorm.DB{}
}

// Where mocks the Where method of gorm.DB
func (m *MockDB) Where(query interface{}, args ...interface{}) *gorm.DB {
	m.Called(query, args)
	return &gorm.DB{}
}

// First mocks the First method of gorm.DB
func (m *MockDB) First(dest interface{}, conds ...interface{}) *gorm.DB {
	m.Called(dest, conds)
	return &gorm.DB{}
}

// Save mocks the Save method of gorm.DB
func (m *MockDB) Save(value interface{}) *gorm.DB {
	m.Called(value)
	return &gorm.DB{}
}

// Find mocks the Find method of gorm.DB
func (m *MockDB) Find(dest interface{}, conds ...interface{}) *gorm.DB {
	m.Called(dest, conds)
	return &gorm.DB{}
}

// Create mocks the Create method of gorm.DB
func (m *MockDB) Create(value interface{}) *gorm.DB {
	m.Called(value)
	return &gorm.DB{}
}

// Delete mocks the Delete method of gorm.DB
func (m *MockDB) Delete(value interface{}, conds ...interface{}) *gorm.DB {
	m.Called(value, conds)
	return &gorm.DB{}
}

// Model mocks the Model method of gorm.DB
func (m *MockDB) Model(value interface{}) *gorm.DB {
	m.Called(value)
	return &gorm.DB{}
}

// Updates mocks the Updates method of gorm.DB
func (m *MockDB) Updates(values interface{}) *gorm.DB {
	m.Called(values)
	return &gorm.DB{}
}

func TestEvaluateSessionRisk(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	mockDB := new(MockDB)
	db := NewMockDB()
	handler := NewSessionSecurityHandler(db)

	// Create a test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	
	// Mock the session in the context
	sessionID := uuid.New()
	c.Set("session_id", sessionID.String())
	
	// Mock the request
	req := httptest.NewRequest("GET", "/api/security/sessions/risk", nil)
	c.Request = req
	
	// Mock the database response
	mockDB.On("Where", "id = ?", []interface{}{sessionID}).Return(mockDB)
	
	// Create a mock session
	session := &database.EnhancedSession{
		ID:           sessionID,
		UserID:       uuid.New(),
		RefreshToken: "test-token",
		UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		IPAddress:    "192.168.1.1",
		Status:       database.SessionStatusActive,
		CreatedAt:    time.Now().Add(-24 * time.Hour),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		LastActiveAt: time.Now(),
		RiskScore:    0,
		RiskLevel:    "low",
	}
	
	// Mock the First method to return our session
	mockDB.On("First", mock.AnythingOfType("*database.EnhancedSession"), []interface{}{}).
		Run(func(args mock.Arguments) {
			arg := args.Get(0).(*database.EnhancedSession)
			*arg = *session
		}).
		Return(&gorm.DB{})
	
	// Mock the Save method
	mockDB.On("Save", mock.AnythingOfType("*database.EnhancedSession")).Return(&gorm.DB{})
	
	// Call the handler
	handler.EvaluateSessionRisk(c)
	
	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	
	// Parse the response
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	// Check the response
	assert.Contains(t, response, "risk_score")
	assert.Contains(t, response, "risk_level")
	assert.Contains(t, response, "risk_factors")
}

func TestVerifySessionSecurity(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	mockDB := new(MockDB)
	db := NewMockDB()
	handler := NewSessionSecurityHandler(db)

	// Create a test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	
	// Mock the session in the context
	sessionID := uuid.New()
	userID := uuid.New()
	c.Set("session_id", sessionID.String())
	c.Set("user_id", userID.String())
	
	// Create request body
	requestBody := map[string]interface{}{
		"verification_type": "mfa",
		"verification_code": "123456",
	}
	jsonBody, _ := json.Marshal(requestBody)
	
	// Mock the request
	req := httptest.NewRequest("POST", "/api/security/sessions/verify", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	
	// Mock the database response
	mockDB.On("Where", "id = ?", []interface{}{sessionID}).Return(mockDB)
	
	// Create a mock session
	session := &database.EnhancedSession{
		ID:           sessionID,
		UserID:       userID,
		RefreshToken: "test-token",
		UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		IPAddress:    "192.168.1.1",
		Status:       database.SessionStatusSuspicious,
		CreatedAt:    time.Now().Add(-24 * time.Hour),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		LastActiveAt: time.Now(),
		RiskScore:    75.0,
		RiskLevel:    "high",
	}
	
	// Mock the First method to return our session
	mockDB.On("First", mock.AnythingOfType("*database.EnhancedSession"), []interface{}{}).
		Run(func(args mock.Arguments) {
			arg := args.Get(0).(*database.EnhancedSession)
			*arg = *session
		}).
		Return(&gorm.DB{})
	
	// Mock the Save method
	mockDB.On("Save", mock.AnythingOfType("*database.EnhancedSession")).Return(&gorm.DB{})
	
	// Mock the user lookup
	mockDB.On("Where", "id = ?", []interface{}{userID}).Return(mockDB)
	mockUser := &database.User{
		ID:    userID,
		Email: "test@example.com",
	}
	mockDB.On("First", mock.AnythingOfType("*database.User"), []interface{}{}).
		Run(func(args mock.Arguments) {
			arg := args.Get(0).(*database.User)
			*arg = *mockUser
		}).
		Return(&gorm.DB{})
	
	// Mock MFA verification (this would normally be in the MFA handler)
	// For testing, we'll just assume it's successful
	
	// Call the handler
	handler.VerifySessionSecurity(c)
	
	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	
	// Parse the response
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	// Check the response
	assert.Contains(t, response, "success")
	assert.Equal(t, true, response["success"])
}

func TestRevokeRiskySessions(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	mockDB := new(MockDB)
	db := NewMockDB()
	handler := NewSessionSecurityHandler(db)

	// Create a test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	
	// Mock the session in the context
	sessionID := uuid.New()
	userID := uuid.New()
	c.Set("session_id", sessionID.String())
	c.Set("user_id", userID.String())
	
	// Create request body
	requestBody := map[string]interface{}{
		"risk_threshold": "high",
	}
	jsonBody, _ := json.Marshal(requestBody)
	
	// Mock the request
	req := httptest.NewRequest("POST", "/api/security/sessions/revoke-risky", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	
	// Mock the database response for finding risky sessions
	mockDB.On("Where", "user_id = ? AND risk_level IN (?) AND id != ?", 
		[]interface{}{userID, []string{"high", "critical"}, sessionID}).Return(mockDB)
	
	// Create mock sessions
	sessions := []database.EnhancedSession{
		{
			ID:        uuid.New(),
			UserID:    userID,
			Status:    database.SessionStatusActive,
			RiskScore: 80.0,
			RiskLevel: "high",
		},
		{
			ID:        uuid.New(),
			UserID:    userID,
			Status:    database.SessionStatusActive,
			RiskScore: 95.0,
			RiskLevel: "critical",
		},
	}
	
	// Mock the Find method to return our sessions
	mockDB.On("Find", mock.AnythingOfType("*[]database.EnhancedSession"), []interface{}{}).
		Run(func(args mock.Arguments) {
			arg := args.Get(0).(*[]database.EnhancedSession)
			*arg = sessions
		}).
		Return(&gorm.DB{})
	
	// Mock the Model and Updates methods for each session
	mockDB.On("Model", mock.AnythingOfType("*database.EnhancedSession")).Return(mockDB)
	mockDB.On("Updates", mock.Anything).Return(&gorm.DB{})
	
	// Call the handler
	handler.RevokeRiskySessions(c)
	
	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	
	// Parse the response
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	// Check the response
	assert.Contains(t, response, "revoked_count")
	assert.Equal(t, float64(2), response["revoked_count"])
}

func TestGetSessionSecurityHistory(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	mockDB := new(MockDB)
	db := NewMockDB()
	handler := NewSessionSecurityHandler(db)

	// Create a test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	
	// Mock the session in the context
	sessionID := uuid.New()
	userID := uuid.New()
	c.Set("session_id", sessionID.String())
	c.Set("user_id", userID.String())
	
	// Set up route params
	c.Params = []gin.Param{
		{
			Key:   "id",
			Value: sessionID.String(),
		},
	}
	
	// Mock the request
	req := httptest.NewRequest("GET", "/api/security/sessions/"+sessionID.String()+"/security-history", nil)
	c.Request = req
	
	// Mock the database response
	mockDB.On("Where", "id = ?", []interface{}{sessionID}).Return(mockDB)
	
	// Create a mock session
	session := &database.EnhancedSession{
		ID:           sessionID,
		UserID:       userID,
		RefreshToken: "test-token",
		UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		IPAddress:    "192.168.1.1",
		Status:       database.SessionStatusActive,
		CreatedAt:    time.Now().Add(-24 * time.Hour),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		LastActiveAt: time.Now(),
		RiskScore:    25.0,
		RiskLevel:    "medium",
	}
	
	// Mock the First method to return our session
	mockDB.On("First", mock.AnythingOfType("*database.EnhancedSession"), []interface{}{}).
		Run(func(args mock.Arguments) {
			arg := args.Get(0).(*database.EnhancedSession)
			*arg = *session
		}).
		Return(&gorm.DB{})
	
	// Mock the audit log lookup
	mockDB.On("Where", "session_id = ?", []interface{}{sessionID}).Return(mockDB)
	
	// Create mock audit logs
	// Convert string IDs to UUID pointers
	userUUID := userID
	sessionUUID := sessionID
	auditLogs := []utils.AuditLog{
		{
			ID:          uuid.New(),
			UserID:      &userUUID,
			EventType:   utils.AuditEventType("session.risk_evaluated"),
			Severity:    utils.AuditSeverityInfo,
			Description: "Session risk evaluated",
			SessionID:   &sessionUUID,
			Timestamp:   time.Now().Add(-1 * time.Hour),
			IPAddress:   "192.168.1.1",
			UserAgent:   "Mozilla/5.0",
			Details:     `{"risk_score": 25.0, "risk_level": "medium", "risk_factors": ["location_change"]}`,
			Success:     true,
		},
		{
			ID:          uuid.New(),
			UserID:      &userUUID,
			EventType:   utils.AuditEventType("session.verified"),
			Severity:    utils.AuditSeverityInfo,
			Description: "Session verified",
			SessionID:   &sessionUUID,
			Timestamp:   time.Now().Add(-30 * time.Minute),
			IPAddress:   "192.168.1.1",
			UserAgent:   "Mozilla/5.0",
			Details:     `{"verification_type": "mfa", "success": true}`,
			Success:     true,
		},
	}
	
	// Mock the Find method to return our audit logs
	mockDB.On("Find", mock.AnythingOfType("*[]utils.AuditLog"), []interface{}{}).
		Run(func(args mock.Arguments) {
			arg := args.Get(0).(*[]utils.AuditLog)
			*arg = auditLogs
		}).
		Return(&gorm.DB{})
	
	// Call the handler
	handler.GetSessionSecurityHistory(c)
	
	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	
	// Parse the response
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	// Check the response
	assert.Contains(t, response, "session")
	assert.Contains(t, response, "audit_logs")
	
	auditLogsResponse := response["audit_logs"].([]interface{})
	assert.Equal(t, 2, len(auditLogsResponse))
}

func TestSessionSecurityMiddleware(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	mockDB := new(MockDB)
	db := NewMockDB()
	handler := NewSessionSecurityHandler(db)

	// Create the middleware
	middleware := handler.SessionSecurityMiddleware()
	
	// Create a test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	
	// Mock the session in the context
	sessionID := uuid.New()
	userID := uuid.New()
	c.Set("session_id", sessionID.String())
	c.Set("user_id", userID.String())
	
	// Mock the request
	req := httptest.NewRequest("GET", "/api/protected", nil)
	c.Request = req
	
	// Mock the database response
	mockDB.On("Where", "id = ?", []interface{}{sessionID}).Return(mockDB)
	
	// Create a mock session with high risk
	session := &database.EnhancedSession{
		ID:           sessionID,
		UserID:       userID,
		RefreshToken: "test-token",
		UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		IPAddress:    "192.168.1.1",
		Status:       database.SessionStatusActive,
		CreatedAt:    time.Now().Add(-24 * time.Hour),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		LastActiveAt: time.Now(),
		RiskScore:    85.0,
		RiskLevel:    "high",
	}
	
	// Mock the First method to return our session
	mockDB.On("First", mock.AnythingOfType("*database.EnhancedSession"), []interface{}{}).
		Run(func(args mock.Arguments) {
			arg := args.Get(0).(*database.EnhancedSession)
			*arg = *session
		}).
		Return(&gorm.DB{})
	
	// Create a handler that will be called if the middleware passes
	handlerCalled := false
	nextHandler := func(c *gin.Context) {
		handlerCalled = true
	}
	
	// Call the middleware
	middleware(c)
	nextHandler(c)
	
	// Assertions
	assert.Equal(t, http.StatusFound, w.Code) // Should redirect to verification
	assert.False(t, handlerCalled) // Next handler should not be called
	
	// Check the Location header for redirect
	location := w.Header().Get("Location")
	assert.Contains(t, location, "/verify-session")
}
