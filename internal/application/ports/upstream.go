// Package ports declares the out ports of the BFF.
package ports

import (
	"context"

	"github.com/IQVO/storefront-bff/internal/domain/composition"
)

// Upstream performs one HTTP call against an upstream service. It returns the
// raw answer for any HTTP status, and an error only when no answer arrived (a
// transport failure, or the context ended).
type Upstream interface {
	Do(ctx context.Context, call composition.Call) (composition.Reply, error)
}
