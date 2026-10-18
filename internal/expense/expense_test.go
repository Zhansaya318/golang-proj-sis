package expense

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   []Transaction
		wantErr error
	}{
		{"valid input", []Transaction{{"food", 100}, {"taxi", 50}}, nil},
		{"nil list", nil, ErrNoTransactions},
		{"empty list", []Transaction{}, ErrNoTransactions},
		{"empty category", []Transaction{{"", 100}}, ErrEmptyCategory},
		{"whitespace category", []Transaction{{"   ", 100}}, ErrEmptyCategory},
		{"zero amount", []Transaction{{"food", 0}}, ErrInvalidAmount},
		{"negative amount", []Transaction{{"food", -5}}, ErrInvalidAmount},
		{"second item invalid", []Transaction{{"food", 100}, {"taxi", -1}}, ErrInvalidAmount},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.input)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestAnalyze(t *testing.T) {
	input := []Transaction{
		{"Food", 2500},
		{"taxi", 1200},
		{" food ", 3000},
		{"cinema", 2800},
	}

	res, err := Analyze(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 4 {
		t.Errorf("Count = %d, want 4", res.Count)
	}
	if res.Total != 9500 {
		t.Errorf("Total = %d, want 9500", res.Total)
	}
	if res.Average != 2375 {
		t.Errorf("Average = %d, want 2375", res.Average)
	}
	if res.Largest != (Transaction{"food", 3000}) {
		t.Errorf("Largest = %+v, want {food 3000}", res.Largest)
	}
	if res.TopCategory != "food" {
		t.Errorf("TopCategory = %q, want food", res.TopCategory)
	}

	wantByCategory := map[string]int{"food": 5500, "taxi": 1200, "cinema": 2800}
	if len(res.ByCategory) != len(wantByCategory) {
		t.Fatalf("ByCategory = %v, want %v", res.ByCategory, wantByCategory)
	}
	for name, want := range wantByCategory {
		if got := res.ByCategory[name]; got != want {
			t.Errorf("ByCategory[%q] = %d, want %d", name, got, want)
		}
	}
}

func TestAnalyzeTopCategoryTieIsDeterministic(t *testing.T) {
	res, err := Analyze([]Transaction{{"taxi", 500}, {"books", 500}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TopCategory != "books" {
		t.Errorf("TopCategory = %q, want books (alphabetically first on a tie)", res.TopCategory)
	}
}

func TestAnalyzeRejectsInvalidInput(t *testing.T) {
	if _, err := Analyze(nil); !errors.Is(err, ErrNoTransactions) {
		t.Fatalf("expected ErrNoTransactions, got %v", err)
	}
}
