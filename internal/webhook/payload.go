package webhook

import (
	"context"
	"errors"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

var errPF09PayloadsNotImplemented = errors.New("payloads not implemented")

// Payloads is the PF-15 builder seam.
type Payloads struct{}

// NewPayloads returns the payload builder placeholder, replaced by PF-15.
func NewPayloads() *Payloads { return &Payloads{} }

// Build is implemented by PF-15 after presenter contracts are available.
func (p *Payloads) Build(ctx context.Context, q store.Querier, ev model.WebhookEvent) ([]byte, error) {
	return nil, errPF09PayloadsNotImplemented
}
