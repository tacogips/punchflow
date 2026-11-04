CREATE TABLE IF NOT EXISTS organization_chart_activation_plans (id TEXT PRIMARY KEY, organization_chart_id TEXT NOT NULL UNIQUE REFERENCES organization_charts(id) ON DELETE CASCADE, due_on TEXT NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS idx_org_chart_activation_plans_due ON organization_chart_activation_plans(due_on);
