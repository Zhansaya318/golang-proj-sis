# Expense Analyzer

SIS project for the **Golang Application Development** course (KBTU, Fall 2026).
Current stage: **Checkpoint 2 — Concurrent Processing** (worker pool, bounded queue, in-memory).

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

## Creating and submitting tasks

```go
svc, err := service.NewWithWorkers(storage.NewMemoryStore(), 3, 5)

t, err := svc.Create(transactions) // validate and store, status pending
err = svc.Submit(t.ID)             // put into the queue, returns immediately
t, err = svc.Get(t.ID)             // current state, can be called at any time
svc.Shutdown()                     // stop accepting tasks, finish queued ones, stop workers
```

`Submit` does not wait for the processing: it only puts the task into the queue.
`Process(id)` from Checkpoint 1 still exists and processes a task in the calling goroutine.

## Worker pool

```
                 queue (chan int, capacity 5)
Submit(id) ────▶ [ id ][ id ][ id ][  ][  ]
                        │
          ┌─────────────┼─────────────┐
          ▼             ▼             ▼
      worker 1      worker 2      worker 3
          │             │             │
          └─────────────┼─────────────┘
                        ▼
            MemoryStore (map + RWMutex)
```

| Setting | Value |
|---|---|
| Workers | 3 (`main.go`) |
| Queue capacity | 5 (`main.go`) |
| Queue type | buffered channel `chan int` of task IDs |
| Full queue | submission is **rejected** with `worker.ErrQueueFull` |

- The pool is in `internal/worker`. `NewPool` starts a fixed number of goroutines, no goroutine is created per task.
- Each worker runs `for id := range queue`, so it keeps taking tasks until the queue is closed and empty.
- The pool calls a `worker.Handler` interface (`Handle(id int)`); `service.Service` implements it.
- Tasks are independent, so the order in which they finish does not matter.

### When the queue is full

`Submit` uses `select` with a `default` case, so it never blocks.
If the channel has no free place, it returns `worker.ErrQueueFull`, and the task is moved
back from `queued` to `pending`. The task is not lost, is never marked as `processing`
and can be submitted again later.

## Task lifecycle

```
Pending ──Submit──▶ Queued ──worker──▶ Processing ──▶ Completed
   ▲                  │                    │
   └── queue full ────┘                    └────────▶ Failed
```

| State | Meaning |
|---|---|
| `pending` | task is created and stored, not submitted yet |
| `queued` | task is accepted and waits in the queue |
| `processing` | a worker is analysing the task |
| `completed` | analysis finished, result is attached to the task |
| `failed` | analysis returned an error; the reason is stored in `Task.Err`, no result |

Allowed transitions:

- `pending → queued` — `Submit` accepted the task;
- `queued → pending` — the queue was full or the pool is stopped (undo of the step above);
- `queued → processing` — a worker took the task;
- `pending → processing` — synchronous `Process` from Checkpoint 1;
- `processing → completed`, `processing → failed`.

`completed` and `failed` are final states. Every other transition is rejected
with `task.ErrInvalidTransition`, for example submitting a task that is already
queued, processing, completed or failed.

## Synchronization

| Shared data | Who uses it | Protected by |
|---|---|---|
| map of tasks, `nextID` | `Create`, `Get`, workers | `sync.RWMutex` in `MemoryStore` |
| fields of each task (status, result, error) | workers write, `Get` reads | the same `RWMutex` |
| queue channel | `Submit` sends, workers receive, `Stop` closes | channel + `sync.Mutex` with a `stopped` flag in `Pool` |
| running workers | `Stop` waits for them | `sync.WaitGroup` |

- `Get` takes a read lock and returns a **copy** of the task, so the caller never reads
  fields that a worker is writing at the same moment.
- State changes are store methods (`Enqueue`, `Start`, `Complete`, `Fail`, `ReturnToPending`).
  Each one checks the current state and changes it under the write lock, as one step.
  So only one goroutine can move a task to `queued` or to `processing` — a task is never processed twice.
- The analysis runs **without** a lock: `Start` returns a copy of the transactions,
  the worker analyses them and then calls `Complete` or `Fail`.
- `Pool.Submit` and `Pool.Stop` hold the pool mutex, so a send on a closed channel is impossible.

## Stopping the workers

`Shutdown()` calls `Pool.Stop()`:

1. sets `stopped = true` and closes the queue channel (the pool owns the channel, only `Stop` closes it);
2. workers finish the tasks that are already in the channel and leave the `range` loop;
3. `sync.WaitGroup.Wait()` waits until every worker goroutine has exited.

After that `Submit` returns `worker.ErrPoolStopped` and the task stays `pending`.
Calling `Shutdown` more than once is safe.

## Project structure

```
expense-analyzer/
├── main.go                     # demo: starts the pool, submits tasks, prints results
└── internal/
    ├── expense/                # domain: Transaction, Result, validation, analysis
    ├── task/                   # Task, Status and allowed state transitions
    ├── storage/                # Store interface + thread-safe in-memory map
    ├── worker/                 # worker pool with a bounded queue
    └── service/                # Create / Get / Process / Submit / Shutdown
```

Main design decisions:

- **`storage.Store` and `service.Analyzer` interfaces** — the service depends on interfaces, so tests can use their own analyzer (for example one that blocks until the test releases it).
- **Pointer vs value receivers** — `*Task` and `*MemoryStore` methods change state, so they use pointer receivers. `Transaction.Validate` and `Status.CanTransitionTo` only read small values, so they use value receivers.
- **Input is copied** when a task is created, and `Get` returns a deep copy (`Task.Clone`), so outside code can not change stored tasks.
- **Errors** are sentinel errors wrapped with context (`fmt.Errorf("...: %w", err)`) and checked with `errors.Is`. `panic` is not used.

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
go test -race ./...
```

Checkpoint 1 tests cover creation, validation, retrieval, processing, results and state transitions.
Checkpoint 2 tests (`internal/worker/pool_test.go`, `internal/service/worker_test.go`,
`internal/storage/storage_test.go`) cover pool startup, asynchronous submission, many tasks with
correct results, two tasks processed at the same time, the worker limit, duplicate and invalid
submissions, a full queue, concurrent `Get`, shutdown, submissions after shutdown and
submit/shutdown running at the same time.

Concurrency tests wait on channels and `sync.WaitGroup`, not on `time.Sleep`.
`time.After` is used only as a timeout, so a broken test fails instead of hanging.
