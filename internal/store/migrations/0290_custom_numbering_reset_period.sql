ALTER TABLE custom_numbering_counters ADD COLUMN last_allocated_at INTEGER;
ALTER TABLE custom_numbering_counters ADD COLUMN last_period TEXT NOT NULL DEFAULT '';
