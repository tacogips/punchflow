package store

import (
	"context"
	"database/sql"
	"fmt"

	"punchflow/internal/model"
)

// FileInsert persists file metadata after its content has been written.
func FileInsert(ctx context.Context, q Querier, file model.File) error {
	_, err := q.ExecContext(ctx, `INSERT INTO files(id,signed_id,filename,content_type,byte_size,checksum,storage_name,uploaded_by,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		file.ID, file.SignedID, file.Filename, file.ContentType, file.ByteSize, file.Checksum, file.StorageName, file.UploadedBy, Millis(file.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: insert file: %w", err)
	}
	return nil
}

// FileBySignedID loads file metadata using its opaque API identifier.
func FileBySignedID(ctx context.Context, q Querier, signedID string) (model.File, error) {
	return pf23LoadFile(ctx, q, `WHERE signed_id=?`, signedID)
}

// FileByID loads file metadata using its internal database identifier.
func FileByID(ctx context.Context, q Querier, id string) (model.File, error) {
	return pf23LoadFile(ctx, q, `WHERE id=?`, id)
}

func pf23LoadFile(ctx context.Context, q Querier, where, value string) (model.File, error) {
	var file model.File
	var uploaded sql.NullString
	var created int64
	err := q.QueryRowContext(ctx, `SELECT id,signed_id,filename,content_type,byte_size,checksum,storage_name,uploaded_by,created_at FROM files `+where, value).
		Scan(&file.ID, &file.SignedID, &file.Filename, &file.ContentType, &file.ByteSize, &file.Checksum, &file.StorageName, &uploaded, &created)
	if err != nil {
		if err == sql.ErrNoRows {
			return file, ErrNotFound
		}
		return file, fmt.Errorf("store: load file: %w", err)
	}
	if uploaded.Valid {
		file.UploadedBy = &uploaded.String
	}
	file.CreatedAt = Time(created)
	return file, nil
}

// FileAttachmentTicketIDs lists tickets referencing a file through any attachment type.
func FileAttachmentTicketIDs(ctx context.Context, q Querier, signedID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT ticket_id FROM (
		SELECT a.ticket_id AS ticket_id FROM file_attachments a JOIN files f ON f.id=a.file_id WHERE f.signed_id=?
		UNION
		SELECT i.ticket_id AS ticket_id FROM ticket_inputs i, json_each(i.ref_ids) refs JOIN files f ON f.signed_id=refs.value WHERE f.signed_id=?
	) ORDER BY ticket_id`, signedID, signedID)
	if err != nil {
		return nil, fmt.Errorf("store: query file attachment tickets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan file attachment ticket: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read file attachment tickets: %w", err)
	}
	return ids, nil
}
