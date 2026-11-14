package present

import (
	"context"
	"fmt"

	"punchflow/internal/store"
)

type WebhookTenantJSON map[string]any

// WebhookTenant returns the public tenant fields included in webhook payloads.
func WebhookTenant(ctx context.Context, q store.Querier) (WebhookTenantJSON, error) {
	tenant, err := store.Tenant(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("present webhook tenant: %w", err)
	}
	return WebhookTenantJSON{"id": tenant.ID, "publicId": tenant.PublicID, "name": tenant.Name}, nil
}
