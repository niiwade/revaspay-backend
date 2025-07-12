package queue

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/go-co-op/gocron"
)

// Worker processes jobs from a queue
type Worker struct {
	redis      *RedisClient
	queue      string
	handler    JobHandler
	numWorkers int
	wg         sync.WaitGroup
	quit       chan struct{}
	scheduler  *gocron.Scheduler
}

// NewWorker creates a new worker
func NewWorker(redis *RedisClient, queue string, handler JobHandler, numWorkers int) *Worker {
	return &Worker{
		redis:      redis,
		queue:      queue,
		handler:    handler,
		numWorkers: numWorkers,
		quit:       make(chan struct{}),
		scheduler:  gocron.NewScheduler(time.UTC),
	}
}

// Start starts the worker
func (w *Worker) Start() {
	log.Printf("Starting %d workers for queue %s", w.numWorkers, w.queue)

	// Start worker goroutines
	for i := 0; i < w.numWorkers; i++ {
		w.wg.Add(1)
		go w.process(i)
	}

	// Start scheduler for recurring jobs
	w.startScheduler()
}

// Stop stops the worker
func (w *Worker) Stop() {
	log.Printf("Stopping workers for queue %s", w.queue)
	close(w.quit)
	w.wg.Wait()
	w.scheduler.Stop()
}

// process processes jobs from the queue
func (w *Worker) process(workerID int) {
	defer w.wg.Done()

	log.Printf("Worker %d for queue %s started", workerID, w.queue)

	for {
		select {
		case <-w.quit:
			log.Printf("Worker %d for queue %s stopped", workerID, w.queue)
			return
		default:
			// Try to get a job from the queue
			job, err := w.redis.Dequeue(w.queue, 1*time.Second)
			if err != nil {
				log.Printf("Error dequeueing job: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}

			// If no job, wait a bit and try again
			if job == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Process the job
			log.Printf("Worker %d processing job %s from queue %s", workerID, job.ID, w.queue)
			ctx := context.Background()

			// Parse the payload for the handler
			var payload interface{}
			if err := json.Unmarshal(job.Payload, &payload); err != nil {
				log.Printf("Error unmarshaling job payload: %v", err)
				if err := w.redis.Fail(w.queue, job, err); err != nil {
					log.Printf("Error marking job %s as failed: %v", job.ID, err)
				}
				continue
			}

			// Execute the handler
			result, err := w.handler(ctx, *job)
			if err != nil {
				log.Printf("Error processing job %s: %v", job.ID, err)
				if err := w.redis.Fail(w.queue, job, err); err != nil {
					log.Printf("Error marking job %s as failed: %v", job.ID, err)
				}
			} else {
				// Mark job as completed
				if err := w.redis.Complete(w.queue, job.ID, result); err != nil {
					log.Printf("Error marking job %s as completed: %v", job.ID, err)
				}
			}
		}
	}
}

// startScheduler starts the scheduler for recurring jobs
func (w *Worker) startScheduler() {
	// Check for recurring jobs every minute
	w.scheduler.Every(1).Minute().Do(func() {
		w.processRecurringJobs()
	})

	// Start the scheduler
	w.scheduler.StartAsync()
}

// processRecurringJobs processes recurring jobs
func (w *Worker) processRecurringJobs() {
	jobs, err := w.redis.GetRecurringJobs()
	if err != nil {
		log.Printf("Error getting recurring jobs: %v", err)
		return
	}

	for _, job := range jobs {
		// Skip disabled jobs or jobs for other queues
		if !job.Enabled || job.Queue != w.queue {
			continue
		}

		// Enqueue the job
		_, err := w.redis.Enqueue(JobType(job.Queue), job.Payload)
		if err != nil {
			log.Printf("Error enqueueing recurring job %s: %v", job.Name, err)
			continue
		}

		// Update last run time
		now := time.Now()
		job.LastRun = &now

		// Save updated job
		data, err := json.Marshal(job)
		if err != nil {
			log.Printf("Error marshaling recurring job %s: %v", job.Name, err)
			continue
		}

		if err := w.redis.client.HSet(w.redis.ctx, recurringPrefix+"jobs", job.Name, data).Err(); err != nil {
			log.Printf("Error updating recurring job %s: %v", job.Name, err)
		}
	}
}

// RegisterHandler registers a handler for a specific job type
func (w *Worker) RegisterHandler(jobType string, handler JobHandler) {
	// This is a simple implementation where we have one handler per worker
	// In a more complex system, we might have multiple handlers per worker
	log.Printf("Registered handler for job type %s", jobType)
}

// WorkerManager manages multiple workers
type WorkerManager struct {
	redis   *RedisClient
	workers map[string]*Worker
	mu      sync.Mutex
}

// NewWorkerManager creates a new worker manager
func NewWorkerManager(redis *RedisClient) *WorkerManager {
	return &WorkerManager{
		redis:   redis,
		workers: make(map[string]*Worker),
	}
}

// RegisterWorker registers a worker for a queue
func (m *WorkerManager) RegisterWorker(queue string, handler JobHandler, numWorkers int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.workers[queue]; exists {
		log.Printf("Worker for queue %s already registered", queue)
		return
	}

	worker := NewWorker(m.redis, queue, handler, numWorkers)
	m.workers[queue] = worker
}

// StartAll starts all registered workers
func (m *WorkerManager) StartAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for queue, worker := range m.workers {
		log.Printf("Starting worker for queue %s", queue)
		worker.Start()
	}
}

// StopAll stops all registered workers
func (m *WorkerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for queue, worker := range m.workers {
		log.Printf("Stopping worker for queue %s", queue)
		worker.Stop()
	}
}

// EnqueueJob enqueues a job to a queue
func (m *WorkerManager) EnqueueJob(queue string, payload interface{}, opts ...EnqueueOption) (string, error) {
	return m.redis.Enqueue(JobType(queue), payload, opts...)
}

// ScheduleJob schedules a job to run after a delay
func (m *WorkerManager) ScheduleJob(queue string, payload interface{}, delay time.Duration, opts ...EnqueueOption) (string, error) {
	options := append(opts, WithDelay(delay))
	return m.redis.Enqueue(JobType(queue), payload, options...)
}

// ScheduleRecurringJob schedules a recurring job
func (m *WorkerManager) ScheduleRecurringJob(name, queue string, payload interface{}, schedule string) error {
	return m.redis.ScheduleRecurring(name, JobType(queue), payload, schedule)
}

// RemoveRecurringJob removes a recurring job
func (m *WorkerManager) RemoveRecurringJob(name string) error {
	return m.redis.RemoveRecurring(name)
}

// GetQueueStats gets statistics for a queue
func (m *WorkerManager) GetQueueStats(queue string) (*QueueStats, error) {
	return m.redis.GetQueueStats(queue)
}
