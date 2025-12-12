package backup

import (
	"AndroidSafeLocal/internal/dedup"
	"AndroidSafeLocal/internal/device"
	"sync"
)

// Job represents a file transfer task
type Job struct {
	SourcePath string
	DestPath   string
	Size       int64
	Timestamp  string
}

// Result represents the outcome of a job
type Result struct {
	Job     Job
	Error   error
	Skipped bool
}

// Processor defines the interface for handling a job
type Processor interface {
	Process(job Job) error
}

// Pool manages a pool of workers
type Pool struct {
	workerCount int
	jobs        chan Job
	results     chan Result
	wg          sync.WaitGroup
	processor   Processor
	registry    *dedup.Registry
}

// NewPool creates a new worker pool
func NewPool(workerCount int, processor Processor, registry *dedup.Registry) *Pool {
	return &Pool{
		workerCount: workerCount,
		jobs:        make(chan Job, 100), // Buffer slightly to avoid blocking main thread immediately
		results:     make(chan Result, 100),
		processor:   processor,
		registry:    registry,
	}
}

// Start launches the workers
func (p *Pool) Start() {
	for i := 0; i < p.workerCount; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// worker processes jobs from the channel
func (p *Pool) worker(id int) {
	defer p.wg.Done()
	for job := range p.jobs {
		// log.Printf("Worker %d starting job: %s\n", id, job.SourcePath)
		err := p.processor.Process(job)
		p.results <- Result{Job: job, Error: err}
	}
}

// AddJob adds a job to the queue
func (p *Pool) AddJob(job Job) {
	// Check de-duplication
	if p.registry != nil {
		f := device.File{
			Path:      job.SourcePath,
			Size:      job.Size,
			Timestamp: job.Timestamp,
		}
		if p.registry.Exists(f) {
			// Skip
			p.results <- Result{Job: job, Error: nil, Skipped: true}
			return
		}
	}
	p.jobs <- job
}

// Close closes the job channel and waits for workers to finish
func (p *Pool) Close() {
	close(p.jobs)
	p.wg.Wait()
	close(p.results)
}

// Results returns the results channel
func (p *Pool) Results() <-chan Result {
	return p.results
}
