package ticketsearch

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"punchflow/internal/access"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestTerms(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", []string{}},
		{"blank", "  \t ", []string{}},
		{"single", "offsite", []string{"offsite"}},
		{"space separated", "a  b\tc", []string{"a", "b", "c"}},
		{"full-width space", "予算　会食", []string{"予算", "会食"}},
		{"leading and trailing", "  a b ", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Terms(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Terms(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}

	t.Run("term count is bounded", func(t *testing.T) {
		got := Terms(strings.Repeat("x ", MaxTerms+5))
		if len(got) != MaxTerms {
			t.Fatalf("len = %d, want %d", len(got), MaxTerms)
		}
	})
	t.Run("term length is bounded in runes", func(t *testing.T) {
		got := Terms(strings.Repeat("あ", MaxTermRunes+10))
		if len([]rune(got[0])) != MaxTermRunes {
			t.Fatalf("rune length = %d, want %d", len([]rune(got[0])), MaxTermRunes)
		}
	})
}

func TestValueText(t *testing.T) {
	if got := valueText(nil); got != "" {
		t.Fatalf("nil -> %q", got)
	}
	if got := valueText("abc"); got != "abc" {
		t.Fatalf("string -> %q", got)
	}
	if got := valueText(12.5); got != "12.5" {
		t.Fatalf("number -> %q", got)
	}
	if got := valueText([]any{"a", "b"}); got != `["a","b"]` {
		t.Fatalf("array -> %q", got)
	}
}

func TestMatchExcludesRestrictedSectionFieldsForEveryCaller(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	definition, err := json.Marshal(model.WorkflowDef{SectionList: []model.WorkflowSection{
		{
			ID: "restricted", SectionType: model.SectionTypeForm, ViewRestriction: true,
			AllowViewAuthor: true,
			FormFields:      []model.FormField{{ID: "secret-field", Code: "secret"}},
		},
		{
			ID: "open", SectionType: model.SectionTypeForm,
			FormFields: []model.FormField{{ID: "public-field", Code: "public"}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO folders(id,code,name,created_at,updated_at) VALUES('folder','folder','Folder',1,1)`, nil},
		{`INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,created_at,updated_at) VALUES('author','author','author@example.test','Author','User','activated','ja','member',1,1)`, nil},
		{`INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('workflow','workflow','visible','folder',?,1,1)`, []any{string(definition)}},
		{`INSERT INTO tickets(id,workflow_id,status,title,author_id,created_at,updated_at) VALUES('ticket','workflow','draft','Searchable title','author',1,1)`, nil},
		{`INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,created_at,updated_at) VALUES('restricted-input','ticket','secret-field','"restrictedneedle"',1,1)`, nil},
		{`INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,created_at,updated_at) VALUES('public-input','ticket','public-field','"publicneedle"',1,1)`, nil},
	}
	for _, statement := range statements {
		if _, err := db.Q().ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed database: %v", err)
		}
	}

	for _, caller := range []struct {
		name       string
		userID     string
		ticketRead bool
	}{
		{name: "authorized section author", userID: "author"},
		{name: "ticket_read administrator", userID: "admin", ticketRead: true},
	} {
		t.Run(caller.name, func(t *testing.T) {
			principal := access.Caller{UserID: caller.userID, TicketRead: caller.ticketRead}
			for keyword, want := range map[string]bool{
				"restrictedneedle": false,
				"publicneedle":     true,
			} {
				got, err := Match(ctx, db.Q(), principal, keyword)
				if err != nil {
					t.Fatalf("Match(%q): %v", keyword, err)
				}
				if got["ticket"] != want {
					t.Errorf("Match(%q) contains ticket = %t, want %t (all matches: %v)", keyword, got["ticket"], want, got)
				}
			}
		})
	}
}
