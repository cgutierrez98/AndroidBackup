package backup

import (
	"AndroidSafeLocal/internal/adb"
	"sync"
)

// RestoreJob represents a file restore task
type RestoreJob struct {
	LocalPath    string // Local file path on PC
	OriginalPath string // Original path on device
	Index        int    // Job index for progress tracking
	Total        int    // Total number of jobs
}

// RestoreResult represents the outcome of a restore job
type RestoreResult struct {
	Job   RestoreJob
	Error error
}

// RestorePool manages parallel restore workers
type RestorePool struct {
	workerCount int
	jobs        chan RestoreJob
	results     chan RestoreResult
	wg          sync.WaitGroup
	client      *adb.Client
}

// NewRestorePool creates a new restore worker pool
func NewRestorePool(workerCount int, client *adb.Client) *RestorePool {
	return &RestorePool{
		workerCount: workerCount,
		jobs:        make(chan RestoreJob, 500),    // Larger buffer for many small files
		results:     make(chan RestoreResult, 500), // Larger buffer to avoid blocking workers
		client:      client,
	}
}

// Start launches the restore workers
func (p *RestorePool) Start() {
	for i := 0; i < p.workerCount; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// worker processes restore jobs from the channel
func (p *RestorePool) worker() {
	defer p.wg.Done()
	for job := range p.jobs {
		err := p.client.Push(job.LocalPath, job.OriginalPath)
		p.results <- RestoreResult{Job: job, Error: err}
	}
}

// AddJob adds a restore job to the queue
func (p *RestorePool) AddJob(job RestoreJob) {
	p.jobs <- job
}

// Close closes the job channel and waits for workers to finish
func (p *RestorePool) Close() {
	close(p.jobs)
	p.wg.Wait()
	close(p.results)
}

// Results returns the results channel
func (p *RestorePool) Results() <-chan RestoreResult {
	return p.results
}
