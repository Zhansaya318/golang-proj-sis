package service

import (
	"errors"

	"expense-analyzer/internal/expense"
	"expense-analyzer/internal/storage"
	"expense-analyzer/internal/task"
)

type Service struct {
	store storage.Store
}

func New(store storage.Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(txs []expense.Transaction) (*task.Task, error) {
	if err := expense.Validate(txs); err != nil {
		return nil, err
	}

	t := task.New(txs)
	if _, err := s.store.Add(t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) Get(id int) (*task.Task, error) {
	return s.store.Get(id)
}

// Process runs the analysis for a pending task.
// On success the task becomes completed and gets a result.
// If the analysis fails, the task becomes failed instead of staying
// stuck in the processing state, and the analysis error is returned.
func (s *Service) Process(id int) (*task.Task, error) {
	t, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}

	if err := t.Start(); err != nil {
		return nil, err
	}

	res, err := expense.Analyze(t.Transactions)
	if err != nil {
		if failErr := t.Fail(err); failErr != nil {
			return nil, errors.Join(err, failErr)
		}
		return t, err
	}

	if err := t.Complete(res); err != nil {
		return nil, err
	}
	return t, nil
}
