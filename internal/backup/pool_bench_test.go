package backup

import (
    "sync/atomic"
    "testing"
)

type benchProcessor struct{
    count int32
}

func (b *benchProcessor) Process(job Job) error {
    atomic.AddInt32(&b.count, 1)
    return nil
}

func BenchmarkPoolProcess(b *testing.B) {
    proc := &benchProcessor{}
    pool := NewPool(10, proc, nil)
    pool.Start()

    var processed int32
    // Start a goroutine to consume results to avoid blocking workers
    done := make(chan struct{})
    go func() {
        for range pool.Results() {
            atomic.AddInt32(&processed, 1)
        }
        close(done)
    }()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        pool.AddJob(Job{SourcePath: "src", DestPath: "dst"})
    }

    // Close and wait workers to finish; results reader will drain results channel
    pool.Close()
    // Wait for reader to finish draining results
    <-done

    if int(atomic.LoadInt32(&processed)) != b.N {
        b.Fatalf("processed %d != queued %d", processed, b.N)
    }
}
