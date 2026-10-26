package composition

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultBudgets(t *testing.T) {
	b := DefaultBudgets()
	if b.For(Query) != 2*time.Second || b.For(Command) != 5*time.Second || b.For(Confirm) != 15*time.Second {
		t.Fatalf("unexpected defaults %+v", b)
	}
	if b.For(Class(99)) != b.Command {
		t.Fatal("unknown class must get the command budget")
	}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBudgetsValidateRejectsNonPositive(t *testing.T) {
	for _, b := range []Budgets{
		{0, time.Second, time.Second}, {time.Second, 0, time.Second}, {time.Second, time.Second, -1},
	} {
		if err := b.Validate(); !errors.Is(err, ErrBadBudget) {
			t.Errorf("%+v: err = %v", b, err)
		}
	}
}

func TestBudgetsValidateEachZeroField(t *testing.T) {
	s := time.Second
	for _, b := range []Budgets{{0, s, s}, {s, 0, s}, {s, s, 0}} {
		if err := b.Validate(); !errors.Is(err, ErrBadBudget) {
			t.Errorf("%+v: err = %v", b, err)
		}
	}
	if err := (Budgets{s, s, s}).Validate(); err != nil {
		t.Errorf("positive budgets: %v", err)
	}
}
