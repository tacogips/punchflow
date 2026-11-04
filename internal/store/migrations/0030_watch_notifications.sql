-- Per-user ticket watch subscriptions (PF-20). A row records an explicit choice:
-- 'watching' (subscribed) or 'unwatched' (opted out, which also suppresses the
-- automatic subscription of workflow-level watchers). Absence of a row means the
-- default applies (workflow-level watchers watch, everyone else does not).
CREATE TABLE IF NOT EXISTS ticket_watches (
  ticket_id TEXT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  state TEXT NOT NULL CHECK(state IN ('watching','unwatched')),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY(ticket_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_ticket_watches_user ON ticket_watches(user_id);

-- Local notification inbox (PF-20). No email or chat delivery exists; rows are
-- written in the same transaction as the ticket event that produced them.
CREATE TABLE IF NOT EXISTS notifications (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  ticket_id TEXT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  comment_id TEXT,
  actor_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  excerpt TEXT NOT NULL DEFAULT '',
  dedupe_key TEXT,
  read_at INTEGER,
  created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_dedupe ON notifications(user_id, dedupe_key) WHERE dedupe_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, read_at, created_at);
CREATE INDEX IF NOT EXISTS idx_notifications_ticket ON notifications(ticket_id);
