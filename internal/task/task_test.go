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
