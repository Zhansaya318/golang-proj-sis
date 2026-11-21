package worker

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const waitLimit = 2 * time.Second

type countingHandler struct {
	mu      sync.Mutex
	handled map[int]int
}

func newCountingHandler() *countingHandler {
	return &countingHandler{handled: make(map[int]int)}
}

func (h *countingHandler) Handle(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handled[id]++
}

func (h *countingHandler) total() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, c := range h.handled {
		n += c
	}
	return n
}

type blockingHandler struct {
	started   chan int
	release   chan struct{}
	active    atomic.Int32
	maxActive atomic.Int32
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{
		started: make(chan int, 100),
		release: make(chan struct{}),
	}
}

func (h *blockingHandler) Handle(id int) {
	n := h.active.Add(1)
	for {
		m := h.maxActive.Load()
		if n <= m || h.maxActive.CompareAndSwap(m, n) {
			break
		}
	}

	h.started <- id
	<-h.release
	h.active.Add(-1)
}

func (h *blockingHandler) waitStarted(t *testing.T) int {
	t.Helper()
	select {
	case id := <-h.started:
		return id
	case <-time.After(waitLimit):
		t.Fatal("timeout waiting for a task to start")
		return 0
	}
}

func newPool(t *testing.T, workers, queueSize int, h Handler) *Pool {
	t.Helper()
	p, err := NewPool(workers, queueSize, h)
	if err != nil {
		t.Fatalf("NewPool: unexpected error: %v", err)
	}
	return p
}

func mustSubmit(t *testing.T, p *Pool, id int) {
	t.Helper()
	if err := p.Submit(id); err != nil {
		t.Fatalf("Submit(%d): unexpected error: %v", id, err)
	}
}

func stopWithTimeout(t *testing.T, p *Pool) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		p.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(waitLimit):
		t.Fatal("Stop did not return: workers did not terminate")
	}
}

func TestNewPoolInvalidConfig(t *testing.T) {
	tests := []struct {
		name      string
		workers   int
		queueSize int
		handler   Handler
		wantErr   error
	}{
		{"zero workers", 0, 5, newCountingHandler(), ErrInvalidConfig},
		{"negative workers", -1, 5, newCountingHandler(), ErrInvalidConfig},
		{"zero queue", 2, 0, newCountingHandler(), ErrInvalidConfig},
		{"nil handler", 2, 5, nil, ErrNilHandler},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewPool(tc.workers, tc.queueSize, tc.handler)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if p != nil {
				t.Error("no pool must be returned on error")
			}
		})
	}
}

func TestPoolProcessesAllSubmittedIDs(t *testing.T) {
	h := newCountingHandler()
	p := newPool(t, 3, 20, h)

	for id := 1; id <= 20; id++ {
		mustSubmit(t, p, id)
	}
	stopWithTimeout(t, p)

	if len(h.handled) != 20 {
		t.Fatalf("handled %d ids, want 20", len(h.handled))
	}
	for id, n := range h.handled {
		if n != 1 {
			t.Errorf("id %d handled %d times, want 1", id, n)
		}
	}
}

func TestPoolRunsTasksConcurrently(t *testing.T) {
	h := newBlockingHandler()
	p := newPool(t, 2, 2, h)

	mustSubmit(t, p, 1)
	mustSubmit(t, p, 2)

	a := h.waitStarted(t)
	b := h.waitStarted(t)
	if a == b {
		t.Fatalf("id %d started twice", a)
	}
	if got := h.active.Load(); got != 2 {
		t.Fatalf("active handlers = %d, want 2", got)
	}

	close(h.release)
	stopWithTimeout(t, p)
}

func TestPoolNeverExceedsWorkerCount(t *testing.T) {
	const workers = 3
	h := newBlockingHandler()
	p := newPool(t, workers, 10, h)

	for id := 1; id <= 10; id++ {
		mustSubmit(t, p, id)
	}
	for i := 0; i < workers; i++ {
		h.waitStarted(t)
	}
	close(h.release)
	stopWithTimeout(t, p)

	if got := h.maxActive.Load(); got != workers {
		t.Fatalf("max parallel handlers = %d, want %d", got, workers)
	}
}

func TestSubmitWhenQueueIsFull(t *testing.T) {
	h := newBlockingHandler()
	p := newPool(t, 1, 1, h)

	mustSubmit(t, p, 1)
	h.waitStarted(t)
	mustSubmit(t, p, 2)

	if err := p.Submit(3); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Submit(3): expected ErrQueueFull, got %v", err)
	}

	close(h.release)
	if got := h.waitStarted(t); got != 2 {
		t.Fatalf("second handled id = %d, want 2", got)
	}
	stopWithTimeout(t, p)
}

func TestSubmitAfterStop(t *testing.T) {
	p := newPool(t, 2, 2, newCountingHandler())
	stopWithTimeout(t, p)

	if err := p.Submit(1); !errors.Is(err, ErrPoolStopped) {
		t.Fatalf("expected ErrPoolStopped, got %v", err)
	}
}

func TestStopTwiceIsSafe(t *testing.T) {
	p := newPool(t, 2, 2, newCountingHandler())
	stopWithTimeout(t, p)
	stopWithTimeout(t, p)
}

func TestStopFinishesAcceptedTasks(t *testing.T) {
	h := newBlockingHandler()
	p := newPool(t, 2, 10, h)

	for id := 1; id <= 10; id++ {
		mustSubmit(t, p, id)
	}
	close(h.release)
	stopWithTimeout(t, p)

	if got := len(h.started); got != 10 {
		t.Fatalf("handled %d tasks, want 10", got)
	}
}

func TestConcurrentSubmitAndStop(t *testing.T) {
	h := newCountingHandler()
	p := newPool(t, 2, 5, h)

	var accepted atomic.Int32
	var wg sync.WaitGroup
	for id := 0; id < 50; id++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := p.Submit(id)
			switch {
			case err == nil:
				accepted.Add(1)
			case errors.Is(err, ErrQueueFull), errors.Is(err, ErrPoolStopped):
			default:
				t.Errorf("Submit(%d): unexpected error: %v", id, err)
			}
		}()
	}
	stopWithTimeout(t, p)
	wg.Wait()

	if got := h.total(); got != int(accepted.Load()) {
		t.Fatalf("handled %d, accepted %d: accepted tasks were lost", got, accepted.Load())
	}
}
