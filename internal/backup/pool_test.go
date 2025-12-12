package backup

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MockProcessor simulates file transfer
type MockProcessor struct {
	ProcessedCount int32
	Failures       map[string]bool
	mu             sync.Mutex
}

func (m *MockProcessor) Process(job Job) error {
	// Simulate work
	time.Sleep(10 * time.Millisecond)

	m.mu.Lock()
	shouldFail := m.Failures[job.SourcePath]
	m.mu.Unlock()

	if shouldFail {
		return errors.New("simulated failure")
	}

	atomic.AddInt32(&m.ProcessedCount, 1)
	return nil
}

func TestPool(t *testing.T) {
	mock := &MockProcessor{
		Failures: make(map[string]bool),
	}

	// Setup 3 failing jobs
	mock.Failures["/data/fail1"] = true
	mock.Failures["/data/fail2"] = true
	mock.Failures["/data/fail3"] = true

	pool := NewPool(5, mock, nil)
	pool.Start()

	// Feed 20 jobs
	totalJobs := 20
	for i := 0; i < totalJobs; i++ {
		// Let's use unique names
		name := "/data/file"
		if i == 0 {
			name = "/data/fail1"
		}
		if i == 1 {
			name = "/data/fail2"
		}
		if i == 2 {
			name = "/data/fail3"
		}

		pool.AddJob(Job{SourcePath: name, DestPath: "/tmp/backup"})
	}

	go func() {
		pool.Close() // Close after sending all? No, usually close after sending.
		// But here we need to ensure all are sent first.
		// pool.AddJob is blocking if buffer is full, but we have buffer 100 and 20 jobs.
		// So safe to close immediately in a goroutine?
		// Actually AddJob is synchronous pushing to channel.
		// If we call Close immediately after loop, it signals "no more jobs".
	}()

	// Wait for results
	successCount := 0
	failCount := 0

	for res := range pool.Results() {
		if res.Error != nil {
			failCount++
		} else {
			successCount++
		}
	}

	if failCount != 3 {
		t.Errorf("Expected 3 failures, got %d", failCount)
	}

	if successCount != 17 {
		t.Errorf("Expected 17 successes, got %d", successCount)
	}

	if atomic.LoadInt32(&mock.ProcessedCount) != 17 {
		t.Errorf("Mock processed count mismatch: %d", mock.ProcessedCount)
	}
}
