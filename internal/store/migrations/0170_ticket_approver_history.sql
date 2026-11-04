CREATE TABLE IF NOT EXISTS ticket_approver_history (
    ticket_id TEXT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (ticket_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_ticket_approver_history_user
    ON ticket_approver_history(user_id, ticket_id);
