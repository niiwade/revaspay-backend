package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// MockJobHandler is a mock implementation of a job handler
type MockJobHandler struct {
	mock.Mock
}

func (m *MockJobHandler) Handle(ctx context.Context, job Job) (interface{}, error) {
	args := m.Called(ctx, job)
	return args.Get(0), args.Error(1)
}

// TestJob represents a simple job payload for testing
type TestJob struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// setupTestDB creates a test database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	// In a real implementation, we would use an in-memory SQLite database
	// but for this test we'll use a mock DB setup
	db, err := gorm.Open(nil, &gorm.Config{})
	require.NoError(t, err)

	// Migrate the job schema
	err = db.AutoMigrate(&Job{})
	require.NoError(t, err)

	return db
}

func TestNewQueue(t *testing.T) {
	db := setupTestDB(t)
	queue := NewQueue(db)

	assert.NotNil(t, queue)
	assert.Equal(t, db, queue.db)
}

func TestEnqueueJob(t *testing.T) {
	db := setupTestDB(t)
	queue := NewQueue(db)

	// Create a test payload
	payload := TestJob{
		ID:      "test-123",
		Message: "Test message",
	}

	// Enqueue the job
	jobID, err := queue.EnqueueJob(JobTypeProcessPayment, payload)

	// Assertions
	assert.NoError(t, err)
	assert.NotEmpty(t, jobID)

	// Verify the job was stored in the database
	var job Job
	err = db.Where("id = ?", jobID).First(&job).Error
	assert.NoError(t, err)
	assert.Equal(t, JobTypeProcessPayment, job.Type)
	assert.Equal(t, JobStatusPending, job.Status)

	// Verify payload was correctly serialized
	var storedPayload TestJob
	err = json.Unmarshal(job.Payload, &storedPayload)
	assert.NoError(t, err)
	assert.Equal(t, payload.ID, storedPayload.ID)
	assert.Equal(t, payload.Message, storedPayload.Message)
}

func TestGetJobByID(t *testing.T) {
	db := setupTestDB(t)
	queue := NewQueue(db)

	// Create and store a job directly in the database
	jobID := uuid.New()
	payload := TestJob{
		ID:      "test-456",
		Message: "Another test message",
	}

	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	job := Job{
		ID:        jobID,
		Type:      JobTypeProcessPayment,
		Payload:   payloadBytes,
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = db.Create(&job).Error
	require.NoError(t, err)

	// GetJob retrieves a job by ID
	retrievedJob, err := queue.GetJob(jobID.String())

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, jobID.String(), retrievedJob.ID.String())
	assert.Equal(t, JobTypeProcessPayment, retrievedJob.Type)
	assert.Equal(t, JobStatusPending, retrievedJob.Status)

	// Verify payload
	var retrievedPayload TestJob
	err = json.Unmarshal(retrievedJob.Payload, &retrievedPayload)
	assert.NoError(t, err)
	assert.Equal(t, payload.ID, retrievedPayload.ID)
	assert.Equal(t, payload.Message, retrievedPayload.Message)
}

func TestUpdateJobStatus(t *testing.T) {
	db := setupTestDB(t)
	queue := NewQueue(db)

	// Create and store a job
	jobID := uuid.New()
	job := Job{
		ID:        jobID,
		Type:      JobTypeProcessPayment,
		Payload:   []byte(`{"test": "data"}`),
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := db.Create(&job).Error
	require.NoError(t, err)

	// Update the job status
	err = queue.UpdateJobStatus(jobID.String(), JobStatusCompleted, nil, nil)
	assert.NoError(t, err)

	// Verify the status was updated
	var updatedJob Job
	err = db.Where("id = ?", jobID).First(&updatedJob).Error
	assert.NoError(t, err)
	assert.Equal(t, JobStatusCompleted, updatedJob.Status)
	assert.Empty(t, updatedJob.Error)

	// Update with an error
	errorMsg := "Test error message"
	err = queue.UpdateJobStatus(jobID.String(), JobStatusFailed, nil, errors.New(errorMsg))
	assert.NoError(t, err)

	// Verify the error was stored
	err = db.Where("id = ?", jobID).First(&updatedJob).Error
	assert.NoError(t, err)
	assert.Equal(t, JobStatusFailed, updatedJob.Status)
	assert.Equal(t, errorMsg, updatedJob.Error)
}

func TestGetPendingJobs(t *testing.T) {
	db := setupTestDB(t)
	// We're not using the queue directly in this test, just the DB
	_ = NewQueue(db)

	// Create multiple jobs with different statuses
	jobs := []Job{
		{
			ID:        uuid.New(),
			Type:      JobTypeProcessPayment,
			Payload:   []byte(`{"id": "1"}`),
			Status:    JobStatusPending,
			CreatedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Type:      JobTypeNotifyPaymentStatus,
			Payload:   []byte(`{"id": "2"}`),
			Status:    JobStatusPending,
			CreatedAt: time.Now().Add(-30 * time.Minute),
			UpdatedAt: time.Now().Add(-30 * time.Minute),
		},
		{
			ID:        uuid.New(),
			Type:      JobTypeProcessPayment,
			Payload:   []byte(`{"id": "3"}`),
			Status:    JobStatusCompleted,
			CreatedAt: time.Now().Add(-2 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Type:      JobTypeProcessPayment,
			Payload:   []byte(`{"id": "4"}`),
			Status:    JobStatusFailed,
			CreatedAt: time.Now().Add(-3 * time.Hour),
			UpdatedAt: time.Now().Add(-2 * time.Hour),
		},
	}

	for _, job := range jobs {
		err := db.Create(&job).Error
		require.NoError(t, err)
	}

	// We'll need to manually query for pending jobs since there's no direct method
	var pendingJobs []Job
	err := db.Where("status = ?", JobStatusPending).Order("created_at asc").Limit(10).Find(&pendingJobs).Error
	assert.NoError(t, err)
	assert.Len(t, pendingJobs, 2)

	// Verify we only got pending jobs
	for _, job := range pendingJobs {
		assert.Equal(t, JobStatusPending, job.Status)
	}

	// Verify they're ordered by creation time (oldest first)
	assert.Equal(t, jobs[0].ID, pendingJobs[0].ID)
	assert.Equal(t, jobs[1].ID, pendingJobs[1].ID)

	// Test limit
	var limitedJobs []Job
	err = db.Where("status = ?", JobStatusPending).Order("created_at asc").Limit(1).Find(&limitedJobs).Error
	assert.NoError(t, err)
	assert.Len(t, limitedJobs, 1)
	assert.Equal(t, jobs[0].ID, limitedJobs[0].ID)
}
