package processing

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeAnalyzer struct {
	mu      sync.Mutex
	calls   int
	lastCus []string
}

func (f *fakeAnalyzer) Analyze(id int64, custom []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastCus = custom
	return nil
}

func TestPoolProcessesAllJobs(t *testing.T) {
	fa := &fakeAnalyzer{}
	p := New(fa, 4)
	p.Start()

	// Re-run scenario: enqueue every id (already-analyzed) with preserved labels.
	for i := int64(1); i <= 4; i++ {
		p.EnqueueWith(i, []string{"keep"})
	}

	// Wait for the pool to drain.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p.Stop(ctx)

	fa.mu.Lock()
	defer fa.mu.Unlock()
	if fa.calls != 4 {
		t.Errorf("expected all 4 jobs to be analyzed, got %d", fa.calls)
	}
	if len(fa.lastCus) != 1 || fa.lastCus[0] != "keep" {
		t.Errorf("custom labels not forwarded to analyzer: %v", fa.lastCus)
	}
}
