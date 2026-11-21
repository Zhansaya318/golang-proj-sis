package task

import (
	"errors"
	"fmt"
	"maps"

	"expense-analyzer/internal/expense"
)

var ErrInvalidTransition = errors.New("invalid state transition")

type Status string

const (
	StatusPending    Status = "pending"
	StatusQueued     Status = "queued"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// queued -> pending is used only when the queue rejects a submitted task.
// pending -> processing is the synchronous Process path from checkpoint 1.
var allowedTransitions = map[Status][]Status{
	StatusPending:    {StatusQueued, StatusProcessing},
	StatusQueued:     {StatusProcessing, StatusPending},
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
	Err          string
}

func New(txs []expense.Transaction) *Task {
	input := make([]expense.Transaction, len(txs))
	copy(input, txs)

	return &Task{
		Transactions: input,
		Status:       StatusPending,
	}
}

// Clone returns a deep copy, so the caller can read it while
// workers keep changing the original.
func (t *Task) Clone() *Task {
	c := *t
	c.Transactions = make([]expense.Transaction, len(t.Transactions))
	copy(c.Transactions, t.Transactions)
	if t.Result != nil {
		res := *t.Result
		res.ByCategory = maps.Clone(t.Result.ByCategory)
		c.Result = &res
	}
	return &c
}

func (t *Task) Enqueue() error {
	return t.transitionTo(StatusQueued)
}

func (t *Task) ReturnToPending() error {
	return t.transitionTo(StatusPending)
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
