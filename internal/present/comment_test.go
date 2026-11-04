package present

import (
	"regexp"
	"testing"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
)

func TestCommentPresenterDeletedAndAttachments(t *testing.T) {
	ctx, db := pf08DB(t)
	q := db.Q()
	now := kftime.ToMillis(time.Date(2026, 3, 4, 5, 6, 7, 123000000, time.UTC))
	if _, err := q.ExecContext(ctx, `INSERT INTO tickets(id,workflow_id,status,current_step,forced_public_type,created_at,updated_at) VALUES('tkc','wf','draft',0,'follow_workflow',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO comments(id,ticket_id,user_id,body,mentioned_user_ids,deleted_at,created_at,updated_at) VALUES('cm','tkc','u1','secret','["u2"]',?,?,?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO files(id,signed_id,filename,content_type,byte_size,checksum,storage_name,uploaded_by,created_at) VALUES('file','signed','doc.pdf','application/pdf',12,'sum','doc','u1',?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO file_attachments(file_id,ticket_id,comment_id,created_at) VALUES('file','tkc','cm',?)`, now); err != nil {
		t.Fatal(err)
	}
	old := AttachmentURL
	AttachmentURL = func(model.File) string { return "https://files.test/doc.pdf" }
	t.Cleanup(func() { AttachmentURL = old })
	got, err := Comment(ctx, q, "cm")
	if err != nil {
		t.Fatal(err)
	}
	if got["body"] != "" || got["deletedAt"] == nil {
		t.Fatalf("deleted comment=%v", got)
	}
	if !regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}\+09:00$`).MatchString(got["createdAt"].(string)) {
		t.Fatalf("createdAt=%v", got["createdAt"])
	}
	attachments := got["attachments"].([]any)
	if len(attachments) != 1 || attachments[0].(map[string]any)["url"] != "https://files.test/doc.pdf" {
		t.Fatalf("attachments=%v", attachments)
	}
	if len(got["mentionedUsers"].([]UserJSON)) != 1 {
		t.Fatalf("mentions=%v", got["mentionedUsers"])
	}
}
