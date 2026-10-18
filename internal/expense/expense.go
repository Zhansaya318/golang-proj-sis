package expense

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrNoTransactions = errors.New("transaction list is empty")
	ErrEmptyCategory  = errors.New("category must not be empty")
	ErrInvalidAmount  = errors.New("amount must be greater than zero")
)

type Transaction struct {
	Category string
	Amount   int
}

func (t Transaction) Validate() error {
	if strings.TrimSpace(t.Category) == "" {
		return ErrEmptyCategory
	}
	if t.Amount <= 0 {
		return ErrInvalidAmount
	}
	return nil
}

type Result struct {
	Count       int
	Total       int
	Average     int
	Largest     Transaction
	ByCategory  map[string]int
	TopCategory string
}

func Validate(txs []Transaction) error {
	if len(txs) == 0 {
		return ErrNoTransactions
	}
	for i, tx := range txs {
		if err := tx.Validate(); err != nil {
			return fmt.Errorf("transaction %d: %w", i, err)
		}
	}
	return nil
}

func Analyze(txs []Transaction) (Result, error) {
	if err := Validate(txs); err != nil {
		return Result{}, err
	}

	res := Result{
		Count:      len(txs),
		ByCategory: make(map[string]int),
	}

	for _, tx := range txs {
		category := normalizeCategory(tx.Category)
		res.Total += tx.Amount
		res.ByCategory[category] += tx.Amount

		if tx.Amount > res.Largest.Amount {
			res.Largest = Transaction{Category: category, Amount: tx.Amount}
		}
	}

	res.Average = res.Total / res.Count
	res.TopCategory = topCategory(res.ByCategory)
	return res, nil
}

func normalizeCategory(c string) string {
	return strings.ToLower(strings.TrimSpace(c))
}

func topCategory(byCategory map[string]int) string {
	names := make([]string, 0, len(byCategory))
	for name := range byCategory {
		names = append(names, name)
	}
	sort.Strings(names)

	top := ""
	for _, name := range names {
		if top == "" || byCategory[name] > byCategory[top] {
			top = name
		}
	}
	return top
}
