package storage

import (
	"errors"
	"fmt"
	"sync"

	"expense-analyzer/internal/expense"
	"expense-analyzer/internal/task"
)

var (
	ErrTaskNotFound = errors.New("task not found")
	ErrNilTask      = errors.New("cannot add nil task")
)

type Store interface {
	Add(t *task.Task) (int, error)
	Get(id int) (*task.Task, error)
	Enqueue(id int) error
	ReturnToPending(id int) error
	Start(id int) ([]expense.Transaction, error)
	Complete(id int, res expense.Result) error
	Fail(id int, reason error) error
}

// MemoryStore can be used by many goroutines at the same time.
// mu protects the map, nextID and the fields of every stored task.
// Tasks are changed only inside MemoryStore methods while mu is held,
// and Get returns a copy, so nobody touches a stored task without the lock.
type MemoryStore struct {
	mu     sync.RWMutex
	tasks  map[int]*task.Task
	nextID int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tasks:  make(map[int]*task.Task),
		nextID: 1,
	}
}

// Add keeps the pointer, so the caller should not change t after Add.
func (s *MemoryStore) Add(t *task.Task) (int, error) {
	if t == nil {
		return 0, ErrNilTask
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t.ID = s.nextID
	s.tasks[t.ID] = t
	s.nextID++
	return t.ID, nil
}

func (s *MemoryStore) Get(id int) (*task.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, err := s.find(id)
	if err != nil {
		return nil, err
	}
	return t.Clone(), nil
}

// The methods below check the current state and change it while holding
// the write lock, so two goroutines can never both make the same transition.

func (s *MemoryStore) Enqueue(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.find(id)
	if err != nil {
		return err
	}
	return t.Enqueue()
}

func (s *MemoryStore) ReturnToPending(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.find(id)
	if err != nil {
		return err
	}
	return t.ReturnToPending()
}

// Start moves the task to processing and returns a copy of its transactions,
// so the analysis can run after the lock is released.
func (s *MemoryStore) Start(id int) ([]expense.Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.find(id)
	if err != nil {
		return nil, err
	}
	if err := t.Start(); err != nil {
		return nil, err
	}

	txs := make([]expense.Transaction, len(t.Transactions))
	copy(txs, t.Transactions)
	return txs, nil
}

func (s *MemoryStore) Complete(id int, res expense.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.find(id)
	if err != nil {
		return err
	}
	return t.Complete(res)
}

func (s *MemoryStore) Fail(id int, reason error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.find(id)
	if err != nil {
		return err
	}
	return t.Fail(reason)
}

// find must be called with mu held.
func (s *MemoryStore) find(id int) (*task.Task, error) {
	t, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("id %d: %w", id, ErrTaskNotFound)
	}
	return t, nil
}
