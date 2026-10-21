package store

import (
	"context"
	"fmt"
)

var truncateOrder = []string{"webhook_attempts", "webhook_events", "webhooks", "file_attachments", "files", "comments", "ticket_links", "ticket_viewers", "ticket_assignees", "ticket_custom_approvers", "ticket_steps", "ticket_slip_items", "ticket_inputs", "tickets", "proxy_approvers", "proxy_applicants", "role_memberships", "roles", "general_master_items", "general_master_fields", "general_masters", "routes", "workflows", "folders", "categories", "team_memberships", "teams", "organization_chart_activation_plans", "organization_charts", "user_invitations", "grades", "api_tokens", "sessions", "oidc_states", "audit_logs", "user_custom_fields", "users", "tenant"}

// TruncateAll deletes application data in FK-safe order, retaining migration history and server secrets.
func TruncateAll(ctx context.Context, q Querier) error {
	for _, table := range truncateOrder {
		if _, err := q.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("store: truncate %s: %w", table, err)
		}
	}
	return nil
}
