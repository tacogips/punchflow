ALTER TABLE ticket_inputs ADD COLUMN reference_snapshot TEXT NOT NULL DEFAULT 'null';
ALTER TABLE ticket_slip_items ADD COLUMN reference_snapshots TEXT NOT NULL DEFAULT '{}';
