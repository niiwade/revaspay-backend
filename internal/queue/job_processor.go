package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// JobProcessorHandler is a function that processes a job's payload
type JobProcessorHandler func(ctx context.Context, job Job) (interface{}, error)

// JobProcessor processes jobs from queues
type JobProcessor struct {
	queue          *RedisQueue
	handlers       map[string]JobProcessorHandler
	workerCount    int
	stopChan       chan struct{}
	wg             sync.WaitGroup
	processingJobs sync.Map
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewJobProcessor creates a new JobProcessor
func NewJobProcessor(queue *RedisQueue, workerCount int) *JobProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	return &JobProcessor{
		queue:       queue,
		handlers:    make(map[string]JobProcessorHandler),
		workerCount: workerCount,
		stopChan:    make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// RegisterHandler registers a handler for a specific queue
func (p *JobProcessor) RegisterHandler(queueName string, handler JobProcessorHandler) {
	p.handlers[queueName] = handler
}

// Start starts the job processor
func (p *JobProcessor) Start() {
	log.Printf("Starting job processor with %d workers", p.workerCount)
	
	// Start workers
	for i := 0; i < p.workerCount; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// Stop stops the job processor
func (p *JobProcessor) Stop() {
	log.Println("Stopping job processor")
	close(p.stopChan)
	p.cancel()
	p.wg.Wait()
	log.Println("Job processor stopped")
}

// worker is a goroutine that processes jobs
func (p *JobProcessor) worker(id int) {
	defer p.wg.Done()
	
	log.Printf("Worker %d started", id)
	
	// List of queues to poll
	queues := make([]string, 0, len(p.handlers))
	for queue := range p.handlers {
		queues = append(queues, queue)
	}
	
	// If no queues are registered, exit
	if len(queues) == 0 {
		log.Printf("Worker %d exiting: no queues registered", id)
		return
	}
	
	// Process jobs until stopped
	for {
		select {
		case <-p.stopChan:
			log.Println("Worker stopping")
			return
		default:
			// Process jobs from each queue
			for _, queueName := range queues {
				// Try to get a job from the queue
				redisJob, err := p.queue.Dequeue(queueName)
				
				if err != nil {
					log.Printf("Worker %d error getting job from queue %s: %v", id, queueName, err)
					continue
				}
				
				if redisJob == nil {
					// No jobs available in this queue
					continue
				}
				
				// Process the job
				jobID := redisJob.ID
				p.processingJobs.Store(jobID, true)
				err = p.ProcessJob(redisJob)
				p.processingJobs.Delete(jobID)
				
				if err != nil {
					log.Printf("Worker %d error processing job %s: %v", id, jobID, err)
				}
				
				// Only process one job per iteration to give other queues a chance
				break
			}
			
			// Sleep briefly to avoid hammering Redis
			time.Sleep(100 * time.Millisecond)
		}
	}
}



// ProcessJob processes a single job
func (p *JobProcessor) ProcessJob(redisJob *RedisJob) error {
	if redisJob == nil {
		return fmt.Errorf("nil job")
	}
	
	// Convert RedisJob to Job for compatibility
	job := redisJob.ConvertToJob()
	
	// Check if we have a handler for this job type
	handler, ok := p.handlers[string(job.Type)]
	if !ok {
		// Mark job as failed
		p.queue.Fail(redisJob.ID, fmt.Errorf("no handler registered for job type: %s", job.Type))
		return fmt.Errorf("no handler registered for job type: %s", job.Type)
	}
	
	// Process the job
	_, err := handler(p.ctx, *job)
	if err != nil {
		// Mark job as failed
		p.queue.Fail(redisJob.ID, err)
		return fmt.Errorf("job processing failed: %w", err)
	}
	
	// Mark job as completed
	p.queue.Complete(redisJob.Queue, redisJob.ID, nil)
	
	return nil
}

// IsProcessing checks if a job is currently being processed
func (p *JobProcessor) IsProcessing(jobID string) bool {
	_, ok := p.processingJobs.Load(jobID)
	return ok
}

// JobPayload is a helper function to unmarshal job payload
func JobPayload(payload []byte, v interface{}) error {
	return json.Unmarshal(payload, v)
}

// JobResult represents the result of a job
type JobResult struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewJobResult creates a new JobResult
func NewJobResult(success bool, message string, data interface{}) *JobResult {
	return &JobResult{
		Success: success,
		Message: message,
		Data:    data,
	}
}

// JobResultBytes serializes a JobResult to JSON
func JobResultBytes(result *JobResult) ([]byte, error) {
	return json.Marshal(result)
}

// ParseJobResult parses a JobResult from JSON
func ParseJobResult(data []byte) (*JobResult, error) {
	var result JobResult
	err := json.Unmarshal(data, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job result: %w", err)
	}
	return &result, nil
}
