package v1

import (
	"context"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// INF-V9-API-02: pf20CheckGeneralMasterQuota enforces tenant plan limits for master creation
// and replacement of a master's custom field collection.
func pf20CheckGeneralMasterQuota(ctx context.Context, q store.Querier, fieldCount int, creating bool) error {
	var plan string
	if err := q.QueryRowContext(ctx, `SELECT plan FROM tenant LIMIT 1`).Scan(&plan); err != nil {
		return kferr.Internal()
	}
	maxMasters, maxFields := 30, 10
	if plan == model.TenantPlanEnterprise {
		maxMasters, maxFields = 100, 30
	}
	if creating {
		var count int
		if err := q.QueryRowContext(ctx, `SELECT count(*) FROM general_masters`).Scan(&count); err != nil {
			return kferr.Internal()
		}
		if count >= maxMasters {
			return kferr.UpgradeRequired()
		}
	}
	if fieldCount > maxFields {
		return kferr.UpgradeRequired()
	}
	return nil
}
