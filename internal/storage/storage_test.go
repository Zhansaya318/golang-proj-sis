package storage

import (
	"errors"
	"sync"
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
	id, err := s.Add(newTask())
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}

	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != id || got.Status != task.StatusPending || got.Transactions[0].Amount != 100 {
		t.Errorf("Get returned %+v", got)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	s := NewMemoryStore()
	id, err := s.Add(newTask())
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}

	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	got.Status = task.StatusCompleted
	got.Transactions[0].Amount = 1

	again, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if again.Status != task.StatusPending || again.Transactions[0].Amount != 100 {
		t.Errorf("changing the returned task changed the stored one: %+v", again)
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

func TestStateChanges(t *testing.T) {
	s := NewMemoryStore()
	id, err := s.Add(newTask())
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}

	if err := s.Enqueue(id); err != nil {
		t.Fatalf("Enqueue: unexpected error: %v", err)
	}
	if err := s.Enqueue(id); !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("second Enqueue: expected ErrInvalidTransition, got %v", err)
	}

	txs, err := s.Start(id)
	if err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}
	if len(txs) != 1 || txs[0].Amount != 100 {
		t.Fatalf("Start returned %v", txs)
	}

	res, err := expense.Analyze(txs)
	if err != nil {
		t.Fatalf("Analyze: unexpected error: %v", err)
	}
	if err := s.Complete(id, res); err != nil {
		t.Fatalf("Complete: unexpected error: %v", err)
	}

	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got.Status != task.StatusCompleted || got.Result == nil || got.Result.Total != 100 {
		t.Errorf("got %+v", got)
	}
}

func TestStartReturnsCopy(t *testing.T) {
	s := NewMemoryStore()
	id, err := s.Add(newTask())
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}

	txs, err := s.Start(id)
	if err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}
	txs[0].Amount = 1

	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got.Transactions[0].Amount != 100 {
		t.Error("changing the slice from Start changed the stored task")
	}
}

func TestStateChangesOnMissingTask(t *testing.T) {
	s := NewMemoryStore()

	if err := s.Enqueue(7); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("Enqueue: expected ErrTaskNotFound, got %v", err)
	}
	if err := s.ReturnToPending(7); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("ReturnToPending: expected ErrTaskNotFound, got %v", err)
	}
	if _, err := s.Start(7); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("Start: expected ErrTaskNotFound, got %v", err)
	}
	if err := s.Complete(7, expense.Result{}); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("Complete: expected ErrTaskNotFound, got %v", err)
	}
	if err := s.Fail(7, errors.New("boom")); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("Fail: expected ErrTaskNotFound, got %v", err)
	}
}

func TestFailAndReturnToPending(t *testing.T) {
	s := NewMemoryStore()
	id, err := s.Add(newTask())
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}

	if err := s.Enqueue(id); err != nil {
		t.Fatalf("Enqueue: unexpected error: %v", err)
	}
	if err := s.ReturnToPending(id); err != nil {
		t.Fatalf("ReturnToPending: unexpected error: %v", err)
	}
	if _, err := s.Start(id); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}
	if err := s.Fail(id, errors.New("boom")); err != nil {
		t.Fatalf("Fail: unexpected error: %v", err)
	}

	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got.Status != task.StatusFailed || got.Err != "boom" {
		t.Errorf("got status %s, err %q", got.Status, got.Err)
	}
}

func TestConcurrentAddGivesUniqueIDs(t *testing.T) {
	s := NewMemoryStore()
	const n = 100

	ids := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := s.Add(newTask())
			if err != nil {
				t.Errorf("Add: unexpected error: %v", err)
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[int]bool)
	for id := range ids {
		if seen[id] {
			t.Fatalf("id %d assigned twice", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Fatalf("got %d ids, want %d", len(seen), n)
	}
}

// Only one of many goroutines may start the same task.
func TestConcurrentStartIsAtomic(t *testing.T) {
	s := NewMemoryStore()
	id, err := s.Add(newTask())
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}

	const n = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0

	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := s.Start(id); err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := s.Get(id); err != nil {
				t.Errorf("Get: unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if succeeded != 1 {
		t.Fatalf("task was started %d times, want exactly 1", succeeded)
	}
}

var _ Store = (*MemoryStore)(nil)
