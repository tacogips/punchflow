package engine

import (
	"fmt"
	"testing"
	"time"

	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

func pf16InsertFile(t *testing.T, e *pf14TestEnv, signed string) {
	t.Helper()
	e.apply(func(tx *store.Tx) error {
		return store.FileInsert(e.ctx, tx, model.File{ID: "file-" + signed, SignedID: signed, Filename: signed + ".txt", ContentType: "text/plain", ByteSize: 3, Checksum: "x", StorageName: signed, CreatedAt: time.Now()})
	})
}

func pf16FileInput(t *testing.T, e *pf14TestEnv, code string, signed ...string) FieldInput {
	t.Helper()
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	in := FieldInput{FormFieldID: fields[code], FormFieldCode: code, Files: signed}
	if len(signed) == 0 {
		in.Files = []string{}
	}
	return in
}

func pf16Attachments(t *testing.T, e *pf14TestEnv, id string) map[string][]string {
	t.Helper()
	detail, err := present.TicketDetail(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, raw := range detail["inputs"].([]any) {
		in := raw.(map[string]any)
		field := in["formField"].(map[string]any)
		for _, a := range in["attachments"].([]any) {
			code := fmt.Sprint(field["code"])
			out[code] = append(out[code], a.(map[string]any)["signedId"].(string))
		}
	}
	for _, raw := range detail["slipItems"].([]any) {
		for _, c := range raw.(map[string]any)["inputs"].([]any) {
			cell := c.(map[string]any)
			for _, a := range cell["attachments"].([]any) {
				code := "slip:" + fmt.Sprint(cell["slipField"].(map[string]any)["code"])
				out[code] = append(out[code], a.(map[string]any)["signedId"].(string))
			}
		}
	}
	return out
}

func pf16AssociationCount(t *testing.T, e *pf14TestEnv, id string) int {
	t.Helper()
	var n int
	if err := e.db.Q().QueryRowContext(e.ctx, `SELECT COUNT(*) FROM file_attachments WHERE ticket_id=? AND comment_id IS NULL`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFileInputsRoundTripAsAttachments(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "expense-claim", func(def *model.WorkflowDef) {
		photo := pf16SlipField("photo", "file")
		photo.Multiple = true
		pf16Section(def, "lines").SlipFields = append(pf16Section(def, "lines").SlipFields, photo)
		for i := range def.SectionList {
			for j := range def.SectionList[i].FormFields {
				if def.SectionList[i].FormFields[j].Code == "receipt" {
					def.SectionList[i].FormFields[j].Multiple = true
				}
			}
		}
	})
	for _, id := range []string{"sig-a", "sig-b", "sig-c"} {
		pf16InsertFile(t, e, id)
	}
	inputs := pf16Inputs(t, e, pf16Base, "receipt")
	inputs = append(inputs, pf16FileInput(t, e, "receipt", "sig-a"))
	row := pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 1})
	row.Inputs = append(row.Inputs, SlipFieldInput{SlipFieldCode: "photo", Files: []string{"sig-b"}})
	id, err := pf16Create(e, "sales-rep", model.TicketStatusDraft, inputs, []SlipItemInput{row})
	if err != nil {
		t.Fatal(err)
	}
	// R06/PF-07: form and detail-row files are presented as attachments and
	// associated with the ticket.
	got := pf16Attachments(t, e, id)
	if fmt.Sprint(got["receipt"]) != "[sig-a]" || fmt.Sprint(got["slip:photo"]) != "[sig-b]" {
		t.Fatalf("attachments=%v, want receipt=[sig-a] photo=[sig-b]", got)
	}
	if n := pf16AssociationCount(t, e, id); n != 2 {
		t.Fatalf("file associations=%d want 2", n)
	}

	// Replacement through the partial form update.
	err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{Inputs: []FieldInput{pf16FileInput(t, e, "receipt", "sig-c", "sig-a")}})
	if err != nil {
		t.Fatal(err)
	}
	got = pf16Attachments(t, e, id)
	if fmt.Sprint(got["receipt"]) != "[sig-c sig-a]" || fmt.Sprint(got["slip:photo"]) != "[sig-b]" {
		t.Fatalf("after replace attachments=%v", got)
	}
	// Removing the files drops the presented attachments and associations.
	err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{
		Inputs:    []FieldInput{pf16FileInput(t, e, "receipt")},
		SlipItems: []SlipItemInput{pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 1})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got = pf16Attachments(t, e, id); len(got) != 0 {
		t.Fatalf("after removal attachments=%v, want none", got)
	}
	if n := pf16AssociationCount(t, e, id); n != 0 {
		t.Fatalf("file associations after removal=%d want 0", n)
	}
	// A file given through `value` instead of `files` is accepted too.
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	if err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{Inputs: []FieldInput{pf14Input("receipt", fields["receipt"], "sig-a")}}); err != nil {
		t.Fatal(err)
	}
	if got = pf16Attachments(t, e, id); fmt.Sprint(got["receipt"]) != "[sig-a]" {
		t.Fatalf("value-supplied file attachments=%v", got)
	}
	// Unknown signed IDs in detail rows are rejected like form files.
	bad := pf16Row("lines", map[string]any{"item": "pen", "qty": 1, "unit-price": 1})
	bad.Inputs = append(bad.Inputs, SlipFieldInput{SlipFieldCode: "photo", Files: []string{"missing"}})
	err = pf16UpdateForm(e, e.actor("sales-rep"), id, FormPatch{SlipItems: []SlipItemInput{bad}})
	assertPF14FieldError(t, err, "validation_failed", "photo")
}

func pf16Links(t *testing.T, e *pf14TestEnv, id string) []string {
	t.Helper()
	rows, err := e.db.Q().QueryContext(e.ctx, `SELECT linked_ticket_id FROM ticket_links WHERE ticket_id=? ORDER BY linked_ticket_id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

func TestAutoLinkTicketFieldsCreateReciprocalLinks(t *testing.T) {
	e := pf14Setup(t)
	pf16Redefine(t, e, "purchase-request", func(def *model.WorkflowDef) {
		for i := range def.SectionList {
			for j := range def.SectionList[i].FormFields {
				if def.SectionList[i].FormFields[j].Code == "related" {
					def.SectionList[i].FormFields[j].AutoLink = true
				}
			}
		}
	})
	a := e.createExpense("sales-rep", 10, false)
	c := e.createExpense("sales-rep", 20, false)
	values := pf14cPurchaseValues(e)
	values["related"] = a
	b, err := pf14cCreate(t, e, "purchase-request", "sales-rep", values, nil)
	if err != nil {
		t.Fatal(err)
	}
	// R20/PF-14: the referenced ticket is linked in both directions.
	if got := pf16Links(t, e, b); fmt.Sprint(got) != fmt.Sprint([]string{a}) {
		t.Fatalf("links of B=%v want [%s]", got, a)
	}
	if got := pf16Links(t, e, a); fmt.Sprint(got) != fmt.Sprint([]string{b}) {
		t.Fatalf("links of A=%v want [%s]", got, b)
	}
	// Re-saving the same reference does not duplicate the link.
	_, fields := pf14FormDefinition(t, e, "purchase-request")
	if err = pf16UpdateFormFor(e, "sales-rep", b, FormPatch{Inputs: []FieldInput{pf14Input("related", fields["related"], a)}}); err != nil {
		t.Fatal(err)
	}
	if got := pf16Links(t, e, b); len(got) != 1 {
		t.Fatalf("links after re-save=%v", got)
	}
	// Changing the reference moves the link.
	if err = pf16UpdateFormFor(e, "sales-rep", b, FormPatch{Inputs: []FieldInput{pf14Input("related", fields["related"], c)}}); err != nil {
		t.Fatal(err)
	}
	if got := pf16Links(t, e, b); fmt.Sprint(got) != fmt.Sprint([]string{c}) {
		t.Fatalf("links of B after change=%v want [%s]", got, c)
	}
	if got := pf16Links(t, e, a); len(got) != 0 {
		t.Fatalf("stale reciprocal link on A: %v", got)
	}
}

func pf16UpdateFormFor(e *pf14TestEnv, actor, id string, patch FormPatch) error {
	return pf16UpdateForm(e, e.actor(actor), id, patch)
}
