ALTER TABLE ticket_assignees ADD COLUMN pending INTEGER NOT NULL DEFAULT 0 CHECK(pending IN (0,1));
