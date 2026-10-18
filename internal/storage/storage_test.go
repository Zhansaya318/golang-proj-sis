package storage

import (
	"errors"
	"testing"

	"expense-analyzer/internal/expense"
	"expense-analyzer/internal/task"
)

func newTask() *task.Task {
	return task.New([]expense.Transaction{{Category: "food", Amount: 100}})
}

func TestAddAssignsUniqueIDs(t *testing.T) {
	s := NewMemoryStore()

	seen := make(map[int]bool)
	for i := 0; i < 5; i++ {
		id, err := s.Add(newTask())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seen[id] {
			t.Fatalf("id %d assigned twice", id)
		}
		seen[id] = true
	}
}

func TestGetReturnsSavedTask(t *testing.T) {
	s := NewMemoryStore()
	tk := newTask()
	id, err := s.Add(tk)
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}

	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != tk {
		t.Error("Get returned a different task")
	}
}

func TestGetMissingTask(t *testing.T) {
	s := NewMemoryStore()

	_, err := s.Get(100)
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestAddNilTask(t *testing.T) {
	if _, err := NewMemoryStore().Add(nil); !errors.Is(err, ErrNilTask) {
		t.Fatalf("expected ErrNilTask, got %v", err)
	}
}

var _ Store = (*MemoryStore)(nil)
