package storage

import (
	"errors"
	"fmt"

	"expense-analyzer/internal/task"
)

var (
	ErrTaskNotFound = errors.New("task not found")
	ErrNilTask      = errors.New("cannot add nil task")
)

type Store interface {
	Add(t *task.Task) (int, error)
	Get(id int) (*task.Task, error)
}

type MemoryStore struct {
	tasks  map[int]*task.Task
	nextID int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tasks:  make(map[int]*task.Task),
		nextID: 1,
	}
}

func (s *MemoryStore) Add(t *task.Task) (int, error) {
	if t == nil {
		return 0, ErrNilTask
	}
	t.ID = s.nextID
	s.tasks[t.ID] = t
	s.nextID++
	return t.ID, nil
}

func (s *MemoryStore) Get(id int) (*task.Task, error) {
	t, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("id %d: %w", id, ErrTaskNotFound)
	}
	return t, nil
}
