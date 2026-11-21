package service

import (
	"errors"

	"expense-analyzer/internal/expense"
	"expense-analyzer/internal/storage"
	"expense-analyzer/internal/task"
	"expense-analyzer/internal/worker"
)

var ErrNoWorkers = errors.New("service was created without workers")

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

func New(store storage.Store) *Service {
	return &Service{store: store, analyzer: expenseAnalyzer{}}
}

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

func (s *Service) Process(id int) (*task.Task, error) {
	return s.run(id)
}

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

func (s *Service) Shutdown() {
	if s.pool != nil {
		s.pool.Stop()
	}
}

func (s *Service) Handle(id int) {
	s.run(id)
}

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
