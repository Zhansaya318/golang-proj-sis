package task

import (
	"errors"
	"fmt"

	"expense-analyzer/internal/expense"
)

var ErrInvalidTransition = errors.New("invalid state transition")

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// allowedTransitions lists every state a task may move to from a given state.
// Completed and Failed are final: they have no outgoing transitions.
var allowedTransitions = map[Status][]Status{
	StatusPending:    {StatusProcessing},
	StatusProcessing: {StatusCompleted, StatusFailed},
}

func (s Status) CanTransitionTo(next Status) bool {
	for _, allowed := range allowedTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

type Task struct {
	ID           int
	Transactions []expense.Transaction
	Status       Status
	Result       *expense.Result
	Err          string // reason of failure, set only when Status is StatusFailed
}

func New(txs []expense.Transaction) *Task {
	input := make([]expense.Transaction, len(txs))
	copy(input, txs)

	return &Task{
		Transactions: input,
		Status:       StatusPending,
	}
}

func (t *Task) Start() error {
	return t.transitionTo(StatusProcessing)
}

func (t *Task) Complete(res expense.Result) error {
	if err := t.transitionTo(StatusCompleted); err != nil {
		return err
	}
	t.Result = &res
	return nil
}

// Fail moves a task that is being processed into the final failed state
// and records why processing failed.
func (t *Task) Fail(reason error) error {
	if err := t.transitionTo(StatusFailed); err != nil {
		return err
	}
	if reason != nil {
		t.Err = reason.Error()
	}
	return nil
}

func (t *Task) transitionTo(next Status) error {
	if !t.Status.CanTransitionTo(next) {
		return fmt.Errorf("task %d: %s -> %s: %w", t.ID, t.Status, next, ErrInvalidTransition)
	}
	t.Status = next
	return nil
}
