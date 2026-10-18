package service

import (
	"errors"
	"testing"

	"expense-analyzer/internal/expense"
	"expense-analyzer/internal/storage"
	"expense-analyzer/internal/task"
)

func newService() *Service {
	return New(storage.NewMemoryStore())
}

var sampleInput = []expense.Transaction{
	{Category: "food", Amount: 2500},
	{Category: "taxi", Amount: 1200},
	{Category: "food", Amount: 3000},
}

// Helpers stop the test with a clear message instead of a nil pointer panic
// when a setup step unexpectedly fails.

func mustCreate(t *testing.T, svc *Service, txs []expense.Transaction) *task.Task {
	t.Helper()
	tk, err := svc.Create(txs)
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	return tk
}

func mustGet(t *testing.T, svc *Service, id int) *task.Task {
	t.Helper()
	tk, err := svc.Get(id)
	if err != nil {
		t.Fatalf("Get(%d): unexpected error: %v", id, err)
	}
	return tk
}

func mustProcess(t *testing.T, svc *Service, id int) *task.Task {
	t.Helper()
	tk, err := svc.Process(id)
	if err != nil {
		t.Fatalf("Process(%d): unexpected error: %v", id, err)
	}
	return tk
}

func TestCreate(t *testing.T) {
	svc := newService()

	tk, err := svc.Create(sampleInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.ID == 0 {
		t.Error("task must get an ID")
	}
	if tk.Status != task.StatusPending {
		t.Errorf("Status = %s, want pending", tk.Status)
	}
	if len(tk.Transactions) != len(sampleInput) {
		t.Errorf("task has %d transactions, want %d", len(tk.Transactions), len(sampleInput))
	}
	if tk.Result != nil {
		t.Error("new task must not have a result")
	}
}

func TestCreateInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		input   []expense.Transaction
		wantErr error
	}{
		{"empty list", nil, expense.ErrNoTransactions},
		{"empty category", []expense.Transaction{{Category: "", Amount: 10}}, expense.ErrEmptyCategory},
		{"negative amount", []expense.Transaction{{Category: "food", Amount: -10}}, expense.ErrInvalidAmount},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newService()

			tk, err := svc.Create(tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if tk != nil {
				t.Error("no task must be returned for invalid input")
			}
			if _, err := svc.Get(1); !errors.Is(err, storage.ErrTaskNotFound) {
				t.Error("invalid task must not be stored")
			}
		})
	}
}

func TestGet(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, sampleInput)

	got, err := svc.Get(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID || got.Status != task.StatusPending {
		t.Errorf("got task %d (%s), want task %d (pending)", got.ID, got.Status, created.ID)
	}
}

func TestGetNotFound(t *testing.T) {
	svc := newService()

	if _, err := svc.Get(999); !errors.Is(err, storage.ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
	if _, err := svc.Process(999); !errors.Is(err, storage.ErrTaskNotFound) {
		t.Fatalf("Process: expected ErrTaskNotFound, got %v", err)
	}
}

func TestProcess(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, sampleInput)

	mustProcess(t, svc, created.ID)
	tk := mustGet(t, svc, created.ID)

	if tk.Status != task.StatusCompleted {
		t.Errorf("Status = %s, want completed", tk.Status)
	}
	if tk.Result == nil {
		t.Fatal("completed task must have a result")
	}

	r := tk.Result
	if r.Total != 6700 {
		t.Errorf("Total = %d, want 6700", r.Total)
	}
	if r.Average != 2233 {
		t.Errorf("Average = %d, want 2233", r.Average)
	}
	if r.ByCategory["food"] != 5500 || r.ByCategory["taxi"] != 1200 {
		t.Errorf("ByCategory = %v, want food:5500 taxi:1200", r.ByCategory)
	}
	if r.TopCategory != "food" {
		t.Errorf("TopCategory = %q, want food", r.TopCategory)
	}
	if r.Largest.Amount != 3000 {
		t.Errorf("Largest = %d, want 3000", r.Largest.Amount)
	}
}

func TestProcessTwiceFails(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, sampleInput)
	first := mustProcess(t, svc, created.ID)
	firstResult := first.Result

	_, err := svc.Process(created.ID)
	if !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}

	tk := mustGet(t, svc, created.ID)
	if tk.Status != task.StatusCompleted {
		t.Errorf("Status = %s, want completed", tk.Status)
	}
	if tk.Result != firstResult {
		t.Error("result must not be replaced by a failed second run")
	}
}

func TestProcessFailedAnalysisMarksTaskFailed(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, sampleInput)

	// Corrupt the stored input after creation so that the analysis fails.
	created.Transactions[0].Amount = -1

	tk, err := svc.Process(created.ID)
	if !errors.Is(err, expense.ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}
	if tk == nil || tk.Status != task.StatusFailed {
		t.Fatalf("task must be failed, got %+v", tk)
	}
	if tk.Result != nil {
		t.Error("failed task must not have a result")
	}
	if tk.Err == "" {
		t.Error("failed task must keep the failure reason")
	}

	if _, err := svc.Process(created.ID); !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("processing a failed task again: expected ErrInvalidTransition, got %v", err)
	}
}

func TestMultipleTasksAreIndependent(t *testing.T) {
	svc := newService()

	a := mustCreate(t, svc, []expense.Transaction{{Category: "food", Amount: 100}})
	b := mustCreate(t, svc, []expense.Transaction{{Category: "books", Amount: 700}, {Category: "books", Amount: 300}})
	c := mustCreate(t, svc, []expense.Transaction{{Category: "taxi", Amount: 50}})

	if a.ID == b.ID || b.ID == c.ID || a.ID == c.ID {
		t.Fatalf("IDs must be unique: %d %d %d", a.ID, b.ID, c.ID)
	}

	mustProcess(t, svc, a.ID)
	mustProcess(t, svc, b.ID)

	gotA := mustGet(t, svc, a.ID)
	gotB := mustGet(t, svc, b.ID)
	gotC := mustGet(t, svc, c.ID)

	if gotA.Result.Total != 100 || gotA.Result.TopCategory != "food" {
		t.Errorf("task A result = %+v", gotA.Result)
	}
	if gotB.Result.Total != 1000 || gotB.Result.TopCategory != "books" {
		t.Errorf("task B result = %+v", gotB.Result)
	}
	if gotC.Status != task.StatusPending || gotC.Result != nil {
		t.Errorf("task C must be untouched, got status %s", gotC.Status)
	}

	gotA.Result.ByCategory["food"] = 0
	if gotB.Result.ByCategory["books"] != 1000 {
		t.Error("changing task A's result affected task B")
	}
}
