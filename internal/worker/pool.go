package worker

import (
	"errors"
	"sync"
)

var (
	ErrInvalidConfig = errors.New("workers and queue size must be greater than zero")
	ErrNilHandler    = errors.New("handler is nil")
	ErrQueueFull     = errors.New("task queue is full")
	ErrPoolStopped   = errors.New("worker pool is stopped")
)

type Handler interface {
	Handle(id int)
}

type Pool struct {
	queue   chan int
	handler Handler

	mu      sync.Mutex
	stopped bool

	wg sync.WaitGroup
}

func NewPool(workers, queueSize int, handler Handler) (*Pool, error) {
	if workers <= 0 || queueSize <= 0 {
		return nil, ErrInvalidConfig
	}
	if handler == nil {
		return nil, ErrNilHandler
	}

	p := &Pool{
		queue:   make(chan int, queueSize),
		handler: handler,
	}

	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go p.work()
	}
	return p, nil
}

func (p *Pool) work() {
	defer p.wg.Done()
	for id := range p.queue {
		p.handler.Handle(id)
	}
}

func (p *Pool) Submit(id int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return ErrPoolStopped
	}

	select {
	case p.queue <- id:
		return nil
	default:
		return ErrQueueFull
	}
}

func (p *Pool) Stop() {
	p.mu.Lock()
	if !p.stopped {
		p.stopped = true
		close(p.queue)
	}
	p.mu.Unlock()

	p.wg.Wait()
}

func (p *Pool) Capacity() int {
	return cap(p.queue)
}
