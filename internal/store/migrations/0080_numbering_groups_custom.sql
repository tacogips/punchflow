CREATE TABLE IF NOT EXISTS ticket_number_key_counters (
    ticket_number_key_id TEXT PRIMARY KEY,
    counter INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS custom_numbering_counters (
    custom_numbering_id TEXT PRIMARY KEY,
    counter INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS ticket_custom_numbering_values (
    ticket_id TEXT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    custom_numbering_id TEXT NOT NULL,
    formatted_number TEXT NOT NULL,
    PRIMARY KEY (ticket_id, custom_numbering_id)
);
