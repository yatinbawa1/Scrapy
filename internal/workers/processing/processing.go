// Package processing runs the AI analysis pipeline in the background using a
// bounded worker pool. The actual analysis is delegated to an Analyzer (the App
// implements it), keeping this package free of domain dependencies.
package processing

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
)

// Analyzer performs the analysis for a single wallpaper id. The custom labels
// are forwarded so a re-analysis can preserve user-defined tags.
type Analyzer interface {
	Analyze(id int64, custom []string) error
}

// Pool processes wallpaper ids concurrently without analyzing the same id twice
// while it is in flight. It supports pausing (so in-flight work can be halted
// for a clean shutdown or on user request) and reports progress stats.
type Pool struct {
	analyzer    Analyzer
	concurrency int
	jobs        chan job
	inflight    map[int64]bool
	mu          sync.Mutex
	wg          sync.WaitGroup
	closed      bool

	paused   bool
	resumeCh chan struct{}
	submitted atomic.Int64
	done     atomic.Int64
}

// job carries a wallpaper id plus optional custom labels to preserve across a
// re-analysis.
type job struct {
	id      int64
	custom  []string
}

func New(analyzer Analyzer, concurrency int) *Pool {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Pool{
		analyzer:    analyzer,
		concurrency: concurrency,
		jobs:        make(chan job, 10000),
		inflight:    make(map[int64]bool),
		resumeCh:    make(chan struct{}),
	}
}

// Start launches the worker goroutines.
func (p *Pool) Start() {
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// Enqueue schedules analysis for id (no-op if already queued/in-flight).
func (p *Pool) Enqueue(id int64) {
	p.EnqueueWith(id, nil)
}

// EnqueueWith schedules analysis for id, forwarding custom labels to preserve
// across a re-analysis (no-op if already queued/in-flight).
func (p *Pool) EnqueueWith(id int64, custom []string) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	if p.inflight[id] {
		p.mu.Unlock()
		return
	}
	p.inflight[id] = true
	p.mu.Unlock()

	select {
	case p.jobs <- job{id: id, custom: custom}:
		p.submitted.Add(1)
	default:
		// Queue full: drop (will be retried on next scrape/analysis pass).
		p.mu.Lock()
		delete(p.inflight, id)
		p.mu.Unlock()
	}
}

// Pause stops workers from pulling new jobs (in-flight work continues until it
// finishes, then workers block until resumed).
func (p *Pool) Pause() {
	p.mu.Lock()
	p.paused = true
	p.mu.Unlock()
}

// Resume unblocks paused workers.
func (p *Pool) Resume() {
	p.mu.Lock()
	if !p.paused {
		p.mu.Unlock()
		return
	}
	p.paused = false
	ch := p.resumeCh
	p.resumeCh = make(chan struct{})
	p.mu.Unlock()
	close(ch)
}

// IsPaused reports whether the pool is paused.
func (p *Pool) IsPaused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paused
}

// Stats returns cumulative submitted, completed and currently-active (queued +
// in-flight) job counts.
func (p *Pool) Stats() (submitted, done, active int64) {
	p.mu.Lock()
	active = int64(len(p.jobs) + len(p.inflight))
	p.mu.Unlock()
	return p.submitted.Load(), p.done.Load(), active
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for {
		p.mu.Lock()
		paused := p.paused
		p.mu.Unlock()
		if paused {
			<-p.resumeCh
			continue
		}
		j, ok := <-p.jobs
		if !ok {
			return
		}
		id := j.id
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[processing] worker recovered: %v", r)
				}
				p.mu.Lock()
				delete(p.inflight, id)
				p.mu.Unlock()
				p.done.Add(1)
			}()
			if err := p.analyzer.Analyze(id, j.custom); err != nil {
				log.Printf("[processing] analyze %d: %v", id, err)
			}
		}()
	}
}

// Stop closes the pool. It does not block indefinitely on in-flight work: if the
// provided context is cancelled (e.g. the app is shutting down) it returns
// promptly so the process can exit without freezing.
func (p *Pool) Stop(ctx context.Context) {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	// Wake any paused workers so they observe the closed channel and exit.
	p.Resume()
	close(p.jobs)
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		// Give up waiting; remaining goroutines end when the process exits.
	}
}
