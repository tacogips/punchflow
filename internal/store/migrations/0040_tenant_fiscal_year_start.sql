ALTER TABLE tenant ADD COLUMN fiscal_year_start_month INTEGER NOT NULL DEFAULT 4 CHECK(fiscal_year_start_month BETWEEN 1 AND 12);
