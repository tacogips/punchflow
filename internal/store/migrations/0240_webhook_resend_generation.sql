ALTER TABLE webhook_events ADD COLUMN manual_resend_generation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE webhook_events ADD COLUMN manual_resend_completed_generation INTEGER NOT NULL DEFAULT 0;
