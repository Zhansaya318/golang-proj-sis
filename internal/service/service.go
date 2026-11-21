package service

import (
	"errors"

	"expense-analyzer/internal/expense"
	"expense-analyzer/internal/storage"
	"expense-analyzer/internal/task"
	"expense-analyzer/internal/worker"
)

var ErrNoWorkers = errors.New("service was created without workers")

// Analyzer does the domain processing of a task.
// The service uses expense.Analyze; tests can pass their own implementation.
type Analyzer interface {
	Analyze(txs []expense.Transaction) (expense.Result, error)
}

type expenseAnalyzer struct{}

func (expenseAnalyzer) Analyze(txs []expense.Transaction) (expense.Result, error) {
	return expense.Analyze(txs)
}

type Service struct {
	store    storage.Store
	analyzer Analyzer
	pool     *worker.Pool
}

// New creates a service without workers: only synchronous Process works.
func New(store storage.Store) *Service {
	return &Service{store: store, analyzer: expenseAnalyzer{}}
}

// NewWithWorkers also starts a worker pool, so tasks can be sent with Submit.
// Shutdown must be called when the service is no longer needed.
func NewWithWorkers(store storage.Store, workers, queueSize int) (*Service, error) {
	s := New(store)
	pool, err := worker.NewPool(workers, queueSize, s)
	if err != nil {
		return nil, err
	}
	s.pool = pool
	return s, nil
}

func (s *Service) Create(txs []expense.Transaction) (*task.Task, error) {
	if err := expense.Validate(txs); err != nil {
		return nil, err
	}

	id, err := s.store.Add(task.New(txs))
	if err != nil {
		return nil, err
	}
	return s.store.Get(id)
}

func (s *Service) Get(id int) (*task.Task, error) {
	return s.store.Get(id)
}

// Process runs the analysis in the calling goroutine (checkpoint 1 behaviour).
func (s *Service) Process(id int) (*task.Task, error) {
	return s.run(id)
}

// Submit puts a pending task into the queue and returns right away,
// a worker processes it later. If the queue is full or the pool is stopped,
// the task goes back to pending and the error is returned.
func (s *Service) Submit(id int) error {
	if s.pool == nil {
		return ErrNoWorkers
	}

	if err := s.store.Enqueue(id); err != nil {
		return err
	}

	if err := s.pool.Submit(id); err != nil {
		if undoErr := s.store.ReturnToPending(id); undoErr != nil {
			return errors.Join(err, undoErr)
		}
		return err
	}
	return nil
}

// Shutdown stops accepting new tasks, waits until the workers finish
// every task that is already queued and until all workers exit.
func (s *Service) Shutdown() {
	if s.pool != nil {
		s.pool.Stop()
	}
}

// Handle is called by the worker goroutines for each task from the queue.
// The result or the failure reason is saved in the task itself.
func (s *Service) Handle(id int) {
	s.run(id)
}

// run analyses a task without holding the store lock.
// store.Start is the guard against double processing:
// only one goroutine can move a task into the processing state.
func (s *Service) run(id int) (*task.Task, error) {
	txs, err := s.store.Start(id)
	if err != nil {
		return nil, err
	}

	res, err := s.analyzer.Analyze(txs)
	if err != nil {
		if failErr := s.store.Fail(id, err); failErr != nil {
			return nil, errors.Join(err, failErr)
		}
		t, getErr := s.store.Get(id)
		if getErr != nil {
			return nil, errors.Join(err, getErr)
		}
		return t, err
	}

	if err := s.store.Complete(id, res); err != nil {
		return nil, err
	}
	return s.store.Get(id)
}
