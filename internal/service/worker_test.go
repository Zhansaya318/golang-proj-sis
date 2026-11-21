package service

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"expense-analyzer/internal/expense"
	"expense-analyzer/internal/storage"
	"expense-analyzer/internal/task"
	"expense-analyzer/internal/worker"
)

const waitLimit = 2 * time.Second

func newAsyncService(t *testing.T, workers, queueSize int) *Service {
	t.Helper()
	svc, err := NewWithWorkers(storage.NewMemoryStore(), workers, queueSize)
	if err != nil {
		t.Fatalf("NewWithWorkers: unexpected error: %v", err)
	}
	t.Cleanup(svc.Shutdown)
	return svc
}

type failingAnalyzer struct{}

func (failingAnalyzer) Analyze([]expense.Transaction) (expense.Result, error) {
	return expense.Result{}, expense.ErrInvalidAmount
}

type countingAnalyzer struct {
	runs atomic.Int32
}

func (a *countingAnalyzer) Analyze(txs []expense.Transaction) (expense.Result, error) {
	a.runs.Add(1)
	return expense.Analyze(txs)
}

type blockingAnalyzer struct {
	started chan struct{}
	release chan struct{}
}

func newBlockingAnalyzer() *blockingAnalyzer {
	return &blockingAnalyzer{
		started: make(chan struct{}, 100),
		release: make(chan struct{}),
	}
}

func (a *blockingAnalyzer) Analyze(txs []expense.Transaction) (expense.Result, error) {
	a.started <- struct{}{}
	<-a.release
	return expense.Analyze(txs)
}

func (a *blockingAnalyzer) waitStarted(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-a.started:
		case <-time.After(waitLimit):
			t.Fatalf("only %d of %d tasks started processing", i, n)
		}
	}
}

func newBlockingService(t *testing.T, workers, queueSize int) (*Service, *blockingAnalyzer) {
	t.Helper()
	svc := newAsyncService(t, workers, queueSize)
	a := newBlockingAnalyzer()
	svc.analyzer = a
	return svc, a
}

func shutdownWithTimeout(t *testing.T, svc *Service) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		svc.Shutdown()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(waitLimit):
		t.Fatal("Shutdown did not return: workers did not terminate")
	}
}

func mustSubmit(t *testing.T, svc *Service, id int) {
	t.Helper()
	if err := svc.Submit(id); err != nil {
		t.Fatalf("Submit(%d): unexpected error: %v", id, err)
	}
}

func checkStatus(t *testing.T, svc *Service, id int, want task.Status) *task.Task {
	t.Helper()
	tk := mustGet(t, svc, id)
	if tk.Status != want {
		t.Fatalf("task %d: Status = %s, want %s", id, tk.Status, want)
	}
	return tk
}

func TestNewWithWorkersInvalidConfig(t *testing.T) {
	if _, err := NewWithWorkers(storage.NewMemoryStore(), 0, 5); !errors.Is(err, worker.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
	if _, err := NewWithWorkers(storage.NewMemoryStore(), 2, 0); !errors.Is(err, worker.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestSubmitDoesNotWaitForProcessing(t *testing.T) {
	svc, an := newBlockingService(t, 2, 5)
	created := mustCreate(t, svc, sampleInput)

	mustSubmit(t, svc, created.ID)
	an.waitStarted(t, 1)
	checkStatus(t, svc, created.ID, task.StatusProcessing)

	close(an.release)
	shutdownWithTimeout(t, svc)

	tk := checkStatus(t, svc, created.ID, task.StatusCompleted)
	if tk.Result == nil || tk.Result.Total != 6700 || tk.Result.TopCategory != "food" {
		t.Fatalf("wrong result: %+v", tk.Result)
	}
}

func TestSubmittedTaskIsQueuedFirst(t *testing.T) {
	svc, an := newBlockingService(t, 1, 5)
	busy := mustCreate(t, svc, sampleInput)
	waiting := mustCreate(t, svc, sampleInput)

	mustSubmit(t, svc, busy.ID)
	an.waitStarted(t, 1)
	mustSubmit(t, svc, waiting.ID)

	checkStatus(t, svc, waiting.ID, task.StatusQueued)

	close(an.release)
	shutdownWithTimeout(t, svc)
	checkStatus(t, svc, waiting.ID, task.StatusCompleted)
}

func TestManyTasksGetCorrectResults(t *testing.T) {
	const n = 30
	svc := newAsyncService(t, 4, n)

	want := make(map[int]int)
	for i := 1; i <= n; i++ {
		tk := mustCreate(t, svc, []expense.Transaction{
			{Category: "food", Amount: i * 100},
			{Category: "taxi", Amount: i},
		})
		want[tk.ID] = i*100 + i
		mustSubmit(t, svc, tk.ID)
	}
	shutdownWithTimeout(t, svc)

	for id, total := range want {
		tk := checkStatus(t, svc, id, task.StatusCompleted)
		if tk.Result.Total != total {
			t.Errorf("task %d: Total = %d, want %d", id, tk.Result.Total, total)
		}
		if tk.Result.TopCategory != "food" {
			t.Errorf("task %d: TopCategory = %q, want food", id, tk.Result.TopCategory)
		}
	}
}

func TestTwoTasksAreProcessedAtTheSameTime(t *testing.T) {
	svc, an := newBlockingService(t, 2, 5)
	a := mustCreate(t, svc, []expense.Transaction{{Category: "food", Amount: 100}})
	b := mustCreate(t, svc, []expense.Transaction{{Category: "books", Amount: 700}})

	mustSubmit(t, svc, a.ID)
	mustSubmit(t, svc, b.ID)

	an.waitStarted(t, 2)
	checkStatus(t, svc, a.ID, task.StatusProcessing)
	checkStatus(t, svc, b.ID, task.StatusProcessing)

	close(an.release)
	shutdownWithTimeout(t, svc)

	gotA := checkStatus(t, svc, a.ID, task.StatusCompleted)
	gotB := checkStatus(t, svc, b.ID, task.StatusCompleted)
	if gotA.Result.Total != 100 || gotB.Result.Total != 700 {
		t.Fatalf("results mixed up: A=%d B=%d", gotA.Result.Total, gotB.Result.Total)
	}
}

func TestSubmitInvalidTask(t *testing.T) {
	svc, an := newBlockingService(t, 1, 5)

	if err := svc.Submit(999); !errors.Is(err, storage.ErrTaskNotFound) {
		t.Fatalf("missing task: expected ErrTaskNotFound, got %v", err)
	}

	running := mustCreate(t, svc, sampleInput)
	queued := mustCreate(t, svc, sampleInput)
	mustSubmit(t, svc, running.ID)
	an.waitStarted(t, 1)
	mustSubmit(t, svc, queued.ID)

	if err := svc.Submit(running.ID); !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("processing task: expected ErrInvalidTransition, got %v", err)
	}
	if err := svc.Submit(queued.ID); !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("queued task: expected ErrInvalidTransition, got %v", err)
	}
	checkStatus(t, svc, running.ID, task.StatusProcessing)
	checkStatus(t, svc, queued.ID, task.StatusQueued)

	close(an.release)
	shutdownWithTimeout(t, svc)

	if err := svc.Submit(running.ID); !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("completed task: expected ErrInvalidTransition, got %v", err)
	}
	checkStatus(t, svc, running.ID, task.StatusCompleted)
}

func TestSubmitCompletedOrFailedTask(t *testing.T) {
	svc := newAsyncService(t, 2, 5)
	done := mustCreate(t, svc, sampleInput)
	mustProcess(t, svc, done.ID)

	svc.analyzer = failingAnalyzer{}
	failed := mustCreate(t, svc, sampleInput)
	if _, err := svc.Process(failed.ID); !errors.Is(err, expense.ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}

	for _, id := range []int{done.ID, failed.ID} {
		if err := svc.Submit(id); !errors.Is(err, task.ErrInvalidTransition) {
			t.Errorf("task %d: expected ErrInvalidTransition, got %v", id, err)
		}
	}
	checkStatus(t, svc, done.ID, task.StatusCompleted)
	checkStatus(t, svc, failed.ID, task.StatusFailed)
}

func TestFailedAnalysisInWorker(t *testing.T) {
	svc := newAsyncService(t, 2, 5)
	svc.analyzer = failingAnalyzer{}
	created := mustCreate(t, svc, sampleInput)
	mustSubmit(t, svc, created.ID)
	shutdownWithTimeout(t, svc)

	tk := checkStatus(t, svc, created.ID, task.StatusFailed)
	if tk.Result != nil || tk.Err == "" {
		t.Fatalf("failed task must have no result and a reason, got %+v", tk)
	}
}

func TestSubmitWhenQueueIsFull(t *testing.T) {
	svc, an := newBlockingService(t, 1, 1)
	a := mustCreate(t, svc, sampleInput)
	b := mustCreate(t, svc, sampleInput)
	c := mustCreate(t, svc, sampleInput)

	mustSubmit(t, svc, a.ID)
	an.waitStarted(t, 1)
	mustSubmit(t, svc, b.ID)

	if err := svc.Submit(c.ID); !errors.Is(err, worker.ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
	checkStatus(t, svc, c.ID, task.StatusPending)

	close(an.release)
	an.waitStarted(t, 1)
	mustSubmit(t, svc, c.ID)
	shutdownWithTimeout(t, svc)

	for _, id := range []int{a.ID, b.ID, c.ID} {
		checkStatus(t, svc, id, task.StatusCompleted)
	}
}

func TestSubmitWithoutWorkers(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, sampleInput)

	if err := svc.Submit(created.ID); !errors.Is(err, ErrNoWorkers) {
		t.Fatalf("expected ErrNoWorkers, got %v", err)
	}
	checkStatus(t, svc, created.ID, task.StatusPending)
	svc.Shutdown()
}

func TestSubmitAfterShutdown(t *testing.T) {
	svc := newAsyncService(t, 2, 5)
	created := mustCreate(t, svc, sampleInput)
	shutdownWithTimeout(t, svc)

	if err := svc.Submit(created.ID); !errors.Is(err, worker.ErrPoolStopped) {
		t.Fatalf("expected ErrPoolStopped, got %v", err)
	}
	checkStatus(t, svc, created.ID, task.StatusPending)
}

func TestShutdownTwice(t *testing.T) {
	svc := newAsyncService(t, 2, 5)
	shutdownWithTimeout(t, svc)
	shutdownWithTimeout(t, svc)
}

func TestShutdownFinishesQueuedTasks(t *testing.T) {
	svc, an := newBlockingService(t, 2, 10)

	ids := make([]int, 0, 10)
	for i := 0; i < 10; i++ {
		tk := mustCreate(t, svc, sampleInput)
		mustSubmit(t, svc, tk.ID)
		ids = append(ids, tk.ID)
	}
	an.waitStarted(t, 2)

	done := make(chan struct{})
	go func() {
		svc.Shutdown()
		close(done)
	}()
	close(an.release)

	select {
	case <-done:
	case <-time.After(waitLimit):
		t.Fatal("Shutdown did not return")
	}
	for _, id := range ids {
		checkStatus(t, svc, id, task.StatusCompleted)
	}
}

func TestSameTaskSubmittedConcurrently(t *testing.T) {
	svc := newAsyncService(t, 4, 50)
	counter := &countingAnalyzer{}
	svc.analyzer = counter
	created := mustCreate(t, svc, sampleInput)

	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.Submit(created.ID)
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, task.ErrInvalidTransition) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	shutdownWithTimeout(t, svc)

	if accepted.Load() != 1 || counter.runs.Load() != 1 {
		t.Fatalf("accepted %d times, analysed %d times, want 1 and 1", accepted.Load(), counter.runs.Load())
	}
	checkStatus(t, svc, created.ID, task.StatusCompleted)
}

func TestGetWhileWorkersAreRunning(t *testing.T) {
	const n = 40
	svc := newAsyncService(t, 4, n)

	ids := make([]int, 0, n)
	for i := 1; i <= n; i++ {
		tk := mustCreate(t, svc, []expense.Transaction{{Category: "food", Amount: i}})
		ids = append(ids, tk.ID)
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for i, id := range ids {
					tk, err := svc.Get(id)
					if err != nil {
						t.Errorf("Get(%d): %v", id, err)
						return
					}
					if tk.Status == task.StatusCompleted && (tk.Result == nil || tk.Result.Total != i+1) {
						t.Errorf("task %d completed with wrong result %+v", id, tk.Result)
						return
					}
				}
			}
		}()
	}

	for _, id := range ids {
		mustSubmit(t, svc, id)
	}
	shutdownWithTimeout(t, svc)
	close(stop)
	readers.Wait()

	for _, id := range ids {
		checkStatus(t, svc, id, task.StatusCompleted)
	}
}

func TestSubmitAndShutdownConcurrently(t *testing.T) {
	svc := newAsyncService(t, 2, 5)

	const n = 40
	ids := make([]int, n)
	for i := range ids {
		ids[i] = mustCreate(t, svc, sampleInput).ID
	}

	accepted := make([]bool, n)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.Submit(id)
			switch {
			case err == nil:
				accepted[i] = true
			case errors.Is(err, worker.ErrQueueFull), errors.Is(err, worker.ErrPoolStopped):
			default:
				t.Errorf("Submit(%d): unexpected error: %v", id, err)
			}
		}()
	}
	shutdownWithTimeout(t, svc)
	wg.Wait()

	for i, id := range ids {
		want := task.StatusPending
		if accepted[i] {
			want = task.StatusCompleted
		}
		checkStatus(t, svc, id, want)
	}
}
