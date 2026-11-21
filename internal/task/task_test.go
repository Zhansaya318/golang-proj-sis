package task

import (
	"errors"
	"testing"

	"expense-analyzer/internal/expense"
)

func TestCanTransitionTo(t *testing.T) {
	tests := []struct {
		from, to Status
		want     bool
	}{
		{StatusPending, StatusProcessing, true},
		{StatusPending, StatusQueued, true},
		{StatusQueued, StatusProcessing, true},
		{StatusQueued, StatusPending, true},
		{StatusQueued, StatusCompleted, false},
		{StatusCompleted, StatusQueued, false},
		{StatusProcessing, StatusQueued, false},
		{StatusProcessing, StatusCompleted, true},
		{StatusProcessing, StatusFailed, true},
		{StatusPending, StatusFailed, false},
		{StatusFailed, StatusProcessing, false},
		{StatusFailed, StatusCompleted, false},
		{StatusCompleted, StatusFailed, false},
		{StatusPending, StatusCompleted, false},
		{StatusCompleted, StatusProcessing, false},
		{StatusCompleted, StatusPending, false},
		{StatusProcessing, StatusPending, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			if got := tc.from.CanTransitionTo(tc.to); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewTaskIsPending(t *testing.T) {
	tk := New([]expense.Transaction{{Category: "food", Amount: 100}})
	if tk.Status != StatusPending {
		t.Errorf("Status = %s, want pending", tk.Status)
	}
	if tk.Result != nil {
		t.Error("new task must not have a result")
	}
}

func TestNewCopiesInput(t *testing.T) {
	input := []expense.Transaction{{Category: "food", Amount: 100}}
	tk := New(input)

	input[0].Amount = 999

	if tk.Transactions[0].Amount != 100 {
		t.Errorf("task input changed to %d, want 100", tk.Transactions[0].Amount)
	}
}

func TestCompleteWithoutStartFails(t *testing.T) {
	tk := New([]expense.Transaction{{Category: "food", Amount: 100}})

	err := tk.Complete(expense.Result{})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if tk.Status != StatusPending || tk.Result != nil {
		t.Errorf("task must stay unchanged, got status %s, result %v", tk.Status, tk.Result)
	}
}

func TestFail(t *testing.T) {
	tk := New([]expense.Transaction{{Category: "food", Amount: 100}})
	if err := tk.Start(); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}

	reason := errors.New("analysis failed")
	if err := tk.Fail(reason); err != nil {
		t.Fatalf("Fail: unexpected error: %v", err)
	}
	if tk.Status != StatusFailed {
		t.Errorf("Status = %s, want failed", tk.Status)
	}
	if tk.Err != reason.Error() {
		t.Errorf("Err = %q, want %q", tk.Err, reason.Error())
	}
	if err := tk.Start(); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("Start on failed task: expected ErrInvalidTransition, got %v", err)
	}
}

func TestFailWithoutStartFails(t *testing.T) {
	tk := New([]expense.Transaction{{Category: "food", Amount: 100}})

	if err := tk.Fail(errors.New("boom")); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if tk.Status != StatusPending || tk.Err != "" {
		t.Errorf("task must stay unchanged, got status %s, err %q", tk.Status, tk.Err)
	}
}

func TestQueuedLifecycle(t *testing.T) {
	tk := New([]expense.Transaction{{Category: "food", Amount: 100}})

	if err := tk.Enqueue(); err != nil {
		t.Fatalf("Enqueue: unexpected error: %v", err)
	}
	if tk.Status != StatusQueued {
		t.Fatalf("Status = %s, want queued", tk.Status)
	}
	if err := tk.Enqueue(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("second Enqueue: expected ErrInvalidTransition, got %v", err)
	}
	if err := tk.Start(); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}
	if err := tk.ReturnToPending(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("processing task must not go back to pending, got %v", err)
	}
}

func TestReturnToPending(t *testing.T) {
	tk := New([]expense.Transaction{{Category: "food", Amount: 100}})

	if err := tk.ReturnToPending(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("pending task: expected ErrInvalidTransition, got %v", err)
	}
	if err := tk.Enqueue(); err != nil {
		t.Fatalf("Enqueue: unexpected error: %v", err)
	}
	if err := tk.ReturnToPending(); err != nil {
		t.Fatalf("ReturnToPending: unexpected error: %v", err)
	}
	if tk.Status != StatusPending {
		t.Errorf("Status = %s, want pending", tk.Status)
	}
}

func TestCloneIsIndependent(t *testing.T) {
	tk := New([]expense.Transaction{{Category: "food", Amount: 100}})
	if err := tk.Start(); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}
	res, err := expense.Analyze(tk.Transactions)
	if err != nil {
		t.Fatalf("Analyze: unexpected error: %v", err)
	}
	if err := tk.Complete(res); err != nil {
		t.Fatalf("Complete: unexpected error: %v", err)
	}

	c := tk.Clone()
	c.Status = StatusPending
	c.Transactions[0].Amount = 1
	c.Result.Total = 0
	c.Result.ByCategory["food"] = 0

	if tk.Status != StatusCompleted || tk.Transactions[0].Amount != 100 {
		t.Errorf("original task changed: %+v", tk)
	}
	if tk.Result.Total != 100 || tk.Result.ByCategory["food"] != 100 {
		t.Errorf("original result changed: %+v", tk.Result)
	}
}
