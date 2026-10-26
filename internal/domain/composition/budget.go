package composition

import (
	"errors"
	"time"
)

// Class is the time-budget class of a route.
type Class int

// Budget classes: queries are quick reads, commands write through one
// upstream, confirm is the purchase saga step that authorizes a payment and
// places the order.
const (
	Query Class = iota
	Command
	Confirm
)

// ErrBadBudget is returned for a non-positive budget.
var ErrBadBudget = errors.New("budget must be positive")

// Budgets maps a class to the whole-request time budget. When it expires the
// edge answers 504; a budget is never retried.
type Budgets struct {
	Query, Command, Confirm time.Duration
}

// DefaultBudgets are 2 s for queries, 5 s for commands and 15 s for confirm.
func DefaultBudgets() Budgets {
	return Budgets{Query: 2 * time.Second, Command: 5 * time.Second, Confirm: 15 * time.Second}
}

// Validate rejects a non-positive budget.
func (b Budgets) Validate() error {
	if b.Query <= 0 || b.Command <= 0 || b.Confirm <= 0 {
		return ErrBadBudget
	}
	return nil
}

// For returns the budget of a class; an unknown class gets the command budget.
func (b Budgets) For(c Class) time.Duration {
	if c == Query {
		return b.Query
	}
	if c == Confirm {
		return b.Confirm
	}
	return b.Command
}
