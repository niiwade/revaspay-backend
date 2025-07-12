package queue

import (
	"encoding/json"
	"fmt"
	"time"
)

// QueueAdapter adapts RedisQueue to implement QueueInterface
type QueueAdapter struct {
	redisQueue *RedisQueue
	handlers   map[JobType]JobHandler
}

// NewQueueAdapter creates a new QueueAdapter
func NewQueueAdapter(redisQueue *RedisQueue) *QueueAdapter {
	return &QueueAdapter{
		redisQueue: redisQueue,
		handlers:   make(map[JobType]JobHandler),
	}
}

// RegisterHandler registers a handler for a job type
func (a *QueueAdapter) RegisterHandler(jobType JobType, handler JobHandler) {
	a.handlers[jobType] = handler
	// Also register with the Redis queue for processing
	a.redisQueue.handlers[jobType] = handler
}

// Enqueue adds a job to the queue
func (a *QueueAdapter) Enqueue(job *Job) error {
	var payload interface{}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal job payload: %w", err)
	}
	
	_, err := a.redisQueue.Enqueue(string(job.Type), payload)
	return err
}

// Dequeue gets a job from the queue
func (a *QueueAdapter) Dequeue(queueName string) (*RedisJob, error) {
	return a.redisQueue.Dequeue(queueName)
}

// Complete marks a job as complete
func (a *QueueAdapter) Complete(queueName string, jobID string, result interface{}) error {
	return a.redisQueue.Complete(queueName, jobID, result)
}

// Fail marks a job as failed
func (a *QueueAdapter) Fail(queueName string, jobID string, err error) error {
	return a.redisQueue.Fail(jobID, err)
}

// Retry retries a job
func (a *QueueAdapter) Retry(queueName string, jobID string, delay int) error {
	return a.redisQueue.Retry(jobID, time.Duration(delay)*time.Second)
}
