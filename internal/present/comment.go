package present

import (
	"context"
	"fmt"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

type CommentJSON map[string]any

// AttachmentURL resolves a file's downloadable URL; the file store assigns it at startup.
var AttachmentURL func(model.File) string = func(model.File) string { return "" }

func Comment(ctx context.Context, q store.Querier, id string) (CommentJSON, error) {
	x, err := store.Comment(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("present comment: %w", err)
	}
	return commentFromModel(ctx, q, x)
}

func Comments(ctx context.Context, q store.Querier, ids []string) ([]CommentJSON, error) {
	items, err := store.Comments(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("present comments: %w", err)
	}
	out := make([]CommentJSON, 0, len(items))
	for _, item := range items {
		v, e := commentFromModel(ctx, q, item)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}

func commentFromModel(ctx context.Context, q store.Querier, x model.Comment) (CommentJSON, error) {
	user, err := User(ctx, q, x.UserID, UserOpts{})
	if err != nil {
		return nil, fmt.Errorf("present comment user: %w", err)
	}
	mentioned, err := Users(ctx, q, x.MentionedUserIDs, UserOpts{})
	if err != nil {
		return nil, fmt.Errorf("present comment mentions: %w", err)
	}
	files, err := store.TicketAttachments(ctx, q, x.TicketID, &x.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("present comment attachments: %w", err)
	}
	attachments := make([]any, 0, len(files))
	for _, file := range files {
		attachments = append(attachments, attachmentJSON(file))
	}
	body := x.Body
	if x.DeletedAt != nil {
		body = ""
	}
	return CommentJSON{"id": x.ID, "body": body, "user": user, "mentionedUsers": mentioned, "attachments": attachments, "deletedAt": nullableTimeValue(x.DeletedAt), "createdAt": pf08Time(x.CreatedAt), "updatedAt": pf08Time(x.UpdatedAt)}, nil
}

func attachmentJSON(file model.File) map[string]any {
	url := ""
	if AttachmentURL != nil {
		url = AttachmentURL(file)
	}
	return map[string]any{"signedId": file.SignedID, "filename": file.Filename, "contentType": file.ContentType, "byteSize": file.ByteSize, "url": url}
}
