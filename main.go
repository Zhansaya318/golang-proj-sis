package main

import (
	"errors"
	"fmt"
	"sort"

	"expense-analyzer/internal/expense"
	"expense-analyzer/internal/service"
	"expense-analyzer/internal/storage"
	"expense-analyzer/internal/task"
	"expense-analyzer/internal/worker"
)

const (
	workers   = 3
	queueSize = 5
)

func main() {
	svc, err := service.NewWithWorkers(storage.NewMemoryStore(), workers, queueSize)
	if err != nil {
		fmt.Println("start error:", err)
		return
	}
	fmt.Printf("Worker pool started: %d workers, queue capacity %d\n\n", workers, queueSize)

	weeks := [][]expense.Transaction{
		{{Category: "Food", Amount: 2500}, {Category: "Taxi", Amount: 1200}, {Category: "food", Amount: 3000}, {Category: "Cinema", Amount: 2800}},
		{{Category: "Books", Amount: 6000}, {Category: "Coffee", Amount: 900}},
		{{Category: "Taxi", Amount: 1500}, {Category: "taxi", Amount: 1700}, {Category: "Food", Amount: 2000}},
		{{Category: "Gym", Amount: 15000}, {Category: "Coffee", Amount: 1200}, {Category: "coffee", Amount: 800}},
	}

	var ids []int
	for _, txs := range weeks {
		t, err := svc.Create(txs)
		if err != nil {
			fmt.Println("create error:", err)
			continue
		}
		if err := svc.Submit(t.ID); err != nil {
			fmt.Printf("submit task %d: %v\n", t.ID, err)
			continue
		}
		ids = append(ids, t.ID)
		fmt.Printf("Task %d submitted\n", t.ID)
	}

	if len(ids) > 0 {
		err = svc.Submit(ids[0])
		fmt.Println("Submit twice:       ", err, "| is ErrInvalidTransition:", errors.Is(err, task.ErrInvalidTransition))
	}

	_, err = svc.Create([]expense.Transaction{{Category: "Food", Amount: -100}})
	fmt.Println("Invalid input:      ", err)

	err = svc.Submit(42)
	fmt.Println("Missing task:       ", err, "| is ErrTaskNotFound:", errors.Is(err, storage.ErrTaskNotFound))

	svc.Shutdown()
	fmt.Println("\nWorkers stopped")

	late, err := svc.Create([]expense.Transaction{{Category: "Food", Amount: 500}})
	if err == nil {
		err = svc.Submit(late.ID)
		fmt.Println("Submit after stop:  ", err, "| is ErrPoolStopped:", errors.Is(err, worker.ErrPoolStopped))
	}
	fmt.Println()

	for _, id := range ids {
		t, err := svc.Get(id)
		if err != nil {
			fmt.Println("get error:", err)
			continue
		}
		printTask(t)
	}
}

func printTask(t *task.Task) {
	if t.Result == nil {
		fmt.Printf("Task %d — %s %s\n\n", t.ID, t.Status, t.Err)
		return
	}

	r := t.Result
	fmt.Printf("Task %d — %s\n", t.ID, t.Status)
	fmt.Printf("  transactions: %d\n", r.Count)
	fmt.Printf("  total:        %d ₸\n", r.Total)
	fmt.Printf("  average:      %d ₸\n", r.Average)
	fmt.Printf("  largest:      %s, %d ₸\n", r.Largest.Category, r.Largest.Amount)
	fmt.Printf("  top category: %s\n", r.TopCategory)

	names := make([]string, 0, len(r.ByCategory))
	for name := range r.ByCategory {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("    %-8s %d ₸\n", name, r.ByCategory[name])
	}
	fmt.Println()
}
