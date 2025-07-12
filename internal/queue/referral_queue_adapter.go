package queue

// ReferralQueueAdapter adapts QueueInterface to implement the Queue interface expected by the referral reward job
type ReferralQueueAdapter struct {
	queueInterface QueueInterface
}

// NewReferralQueueAdapter creates a new adapter that wraps a QueueInterface to be used with referral reward jobs
func NewReferralQueueAdapter(queueInterface QueueInterface) Queue {
	return Queue{
		db:       nil, // Not used by referral reward job
		handlers: make(map[JobType]JobHandler),
	}
}

// RegisterHandler registers a handler for a job type
// This implementation delegates to the underlying QueueInterface
func (q *ReferralQueueAdapter) RegisterHandler(jobType JobType, handler JobHandler) {
	q.queueInterface.RegisterHandler(jobType, handler)
}

// EnqueueJob adds a job to the queue
// This implementation delegates to the underlying QueueInterface
func (q *ReferralQueueAdapter) EnqueueJob(jobType JobType, payload interface{}) (string, error) {
	// Convert to Job
	job := &Job{
		Type: jobType,
	}
	
	// Delegate to the underlying QueueInterface
	return "", q.queueInterface.Enqueue(job)
}
