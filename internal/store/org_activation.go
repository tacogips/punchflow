package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"punchflow/internal/model"
)

// OrgChartActivationPlanByChart returns the pending activation plan of a chart,
// or ErrNotFound when none is scheduled.
func OrgChartActivationPlanByChart(ctx context.Context, q Querier, chartID string) (model.OrgChartActivationPlan, error) {
	var plan model.OrgChartActivationPlan
	var created, updated int64
	err := q.QueryRowContext(ctx, "SELECT id,organization_chart_id,due_on,created_at,updated_at FROM organization_chart_activation_plans WHERE organization_chart_id=?", chartID).
		Scan(&plan.ID, &plan.OrganizationChartID, &plan.DueOn, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return model.OrgChartActivationPlan{}, ErrNotFound
	}
	if err != nil {
		return model.OrgChartActivationPlan{}, fmt.Errorf("store: read activation plan: %w", err)
	}
	plan.CreatedAt, plan.UpdatedAt = Time(created), Time(updated)
	return plan, nil
}

// OrgChartActivationPlansClear removes every scheduled activation. Activating a
// chart (immediately or on schedule) discards the schedules of all charts.
func OrgChartActivationPlansClear(ctx context.Context, q Querier) error {
	if _, err := q.ExecContext(ctx, "DELETE FROM organization_chart_activation_plans"); err != nil {
		return fmt.Errorf("store: clear activation plans: %w", err)
	}
	return nil
}

// OrgChartActivationPlanReplace schedules a chart for activation, replacing any
// previously scheduled plan (only one plan can be pending at a time).
func OrgChartActivationPlanReplace(ctx context.Context, q Querier, plan model.OrgChartActivationPlan) error {
	if err := OrgChartActivationPlansClear(ctx, q); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, "INSERT INTO organization_chart_activation_plans(id,organization_chart_id,due_on,created_at,updated_at) VALUES(?,?,?,?,?)",
		plan.ID, plan.OrganizationChartID, plan.DueOn, Millis(plan.CreatedAt), Millis(plan.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: insert activation plan: %w", err)
	}
	return nil
}

// ApplyDueOrgChartActivation activates the chart whose scheduled date has been
// reached (dueOn <= today, a JST date) and discards all plans. It reports the
// activated chart ID, or "" when nothing was due. It is cheap enough to run
// lazily at the start of every authenticated request.
func ApplyDueOrgChartActivation(ctx context.Context, db *DB, today string, at time.Time) (string, error) {
	var due string
	err := db.Q().QueryRowContext(ctx, "SELECT organization_chart_id FROM organization_chart_activation_plans WHERE due_on<=? ORDER BY due_on ASC, created_at ASC LIMIT 1", today).Scan(&due)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: find due activation plan: %w", err)
	}
	activated := ""
	err = db.WithTx(ctx, func(tx *Tx) error {
		// Re-check inside the transaction so concurrent requests apply a plan once.
		var chart string
		e := tx.QueryRowContext(ctx, "SELECT organization_chart_id FROM organization_chart_activation_plans WHERE due_on<=? ORDER BY due_on ASC, created_at ASC LIMIT 1", today).Scan(&chart)
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return fmt.Errorf("store: confirm due activation plan: %w", e)
		}
		if e = OrgChartActivate(ctx, tx, chart, at); e != nil {
			return e
		}
		if e = OrgChartActivationPlansClear(ctx, tx); e != nil {
			return e
		}
		data, e := json.Marshal(map[string]string{"organizationChartId": chart})
		if e != nil {
			return e
		}
		system := "automation" // system-initiated operation, as in AuditLog.systemType
		if e = InsertAuditLog(ctx, tx, model.AuditLog{Action: "organizationChart.activate", Data: data, SystemType: &system, CreatedAt: at}); e != nil {
			return e
		}
		activated = chart
		return nil
	})
	if err != nil {
		return "", err
	}
	return activated, nil
}
