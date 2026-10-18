# Expense Analyzer

SIS project for the **Golang Application Development** course (KBTU, Fall 2026).
Current stage: **Checkpoint 1 — Core Go** (synchronous, in-memory).

## What it does

The application processes **tasks**. A task is one unit of work:
a list of expense transactions that has to be analyzed.

| | Example |
|---|---|
| **Input** | `[{food, 2500}, {taxi, 1200}, {food, 3000}]` |
| **Processing** | validate the transactions and calculate spending statistics |
| **Result** | total `6700`, average `2233`, largest `food 3000`, per category `food: 5500, taxi: 1200`, top category `food` |

Each result contains:

- number of transactions;
- total amount;
- average amount (rounded down);
- the largest single expense;
- total per category (categories are case-insensitive: `Food` and `food` are the same);
- the top category (on a tie, the alphabetically first one, so the result is deterministic).

Amounts are whole tenge stored as `int`, not `float64`, to avoid floating-point rounding errors with money.

## Validation rules

A task is rejected (an `error` is returned, nothing is stored) if:

- the transaction list is empty → `expense.ErrNoTransactions`;
- a transaction has an empty or whitespace-only category → `expense.ErrEmptyCategory`;
- a transaction amount is zero or negative → `expense.ErrInvalidAmount`.

## Task lifecycle

```
Pending ──Start()──▶ Processing ──Complete()──▶ Completed
                         │
                         └──────Fail()────────▶ Failed
```

| State | Meaning |
|---|---|
| `pending` | task is created and stored, not processed yet |
| `processing` | analysis has started |
| `completed` | analysis finished, result is attached to the task |
| `failed` | analysis returned an error; the reason is stored in `Task.Err`, no result |

Allowed transitions:

- `pending → processing`
- `processing → completed`
- `processing → failed`

`completed` and `failed` are final states. Every other transition is rejected
with `task.ErrInvalidTransition`, for example:

- processing a task that is already `completed` or `failed`;
- completing or failing a task that was never started (`pending → completed`, `pending → failed`).

Input is validated when the task is created, so in normal use the analysis does
not fail. The `failed` state guarantees that a task never gets stuck in
`processing` if the analysis does return an error.

## Project structure

```
expense-analyzer/
├── main.go                     # demo: creates, processes and prints tasks
└── internal/
    ├── expense/                # domain: Transaction, Result, validation, analysis
    ├── task/                   # Task, Status and allowed state transitions
    ├── storage/                # Store interface + in-memory map implementation
    └── service/                # Create / Get / Process — connects everything
```

Main design decisions:

- **`storage.Store` interface** — the service depends on the interface, not on the map-based `MemoryStore`, so the storage can be replaced later without changing the service.
- **Pointer vs value receivers** — `*Task` methods (`Start`, `Complete`) change the task, so they use pointer receivers. `Transaction.Validate` and `Status.CanTransitionTo` only read small values, so they use value receivers.
- **Input is copied** when a task is created, so changing the caller's slice later does not change the task.
- **Errors** are sentinel errors (`ErrNoTransactions`, `ErrEmptyCategory`, `ErrInvalidAmount`, `ErrInvalidTransition`, `ErrTaskNotFound`, `ErrNilTask`) wrapped with context (`fmt.Errorf("...: %w", err)`) and checked with `errors.Is`. `panic` is not used.

## How to run

Requirements: Go 1.27.

```bash
go run .
```

## How to test

```bash
gofmt -l .
go vet ./...
go test ./...
```

Tests cover task creation, invalid input (table-driven), retrieval, missing tasks,
processing, result correctness, state after processing, failed processing,
invalid transitions (table-driven) and multiple independent tasks.
