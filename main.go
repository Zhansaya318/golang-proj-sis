package main

import (
	"errors"
	"fmt"
	"sort"

	"expense-analyzer/internal/expense"
	"expense-analyzer/internal/service"
	"expense-analyzer/internal/storage"
	"expense-analyzer/internal/task"
)

func main() {
	svc := service.New(storage.NewMemoryStore())

	week1, err := svc.Create([]expense.Transaction{
		{Category: "Food", Amount: 2500},
		{Category: "Taxi", Amount: 1200},
		{Category: "food", Amount: 3000},
		{Category: "Cinema", Amount: 2800},
	})
	if err != nil {
		fmt.Println("create error:", err)
		return
	}

	week2, err := svc.Create([]expense.Transaction{
		{Category: "Books", Amount: 6000},
		{Category: "Coffee", Amount: 900},
	})
	if err != nil {
		fmt.Println("create error:", err)
		return
	}

	fmt.Printf("Created task %d (%s) and task %d (%s)\n\n", week1.ID, week1.Status, week2.ID, week2.Status)

	for _, id := range []int{week1.ID, week2.ID} {
		t, err := svc.Process(id)
		if err != nil {
			fmt.Println("process error:", err)
			continue
		}
		printTask(t)
	}

	_, err = svc.Create([]expense.Transaction{{Category: "Food", Amount: -100}})
	fmt.Println("Invalid input:      ", err)

	_, err = svc.Get(42)
	fmt.Println("Missing task:       ", err, "| is ErrTaskNotFound:", errors.Is(err, storage.ErrTaskNotFound))

	_, err = svc.Process(week1.ID)
	fmt.Println("Process twice:      ", err, "| is ErrInvalidTransition:", errors.Is(err, task.ErrInvalidTransition))
}

func printTask(t *task.Task) {
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
