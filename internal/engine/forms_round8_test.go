package engine

import (
	"context"
	"testing"
	"time"

	"punchflow/internal/model"
)

func TestRound8DecimalRoundingBoundsExtremeExponents(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		got := pf14RoundStoredNumeric("number", "1e-400000000")
		if got != "1e-400000000" {
			t.Errorf("extreme value = %v, want original decimal", got)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stored numeric rounding exceeded 2s")
	}
}

func TestRound8CheckboxRequiresArrayAndTextHasGlobalLimit(t *testing.T) {
	checkbox := model.FormField{FieldType: "checkbox", Options: []string{"selected"}}
	if err := pf14cValidateFormValue(context.Background(), nil, checkbox, "selected"); err == nil {
		t.Fatal("scalar checkbox value was accepted")
	}
	if err := pf14cValidateFormValue(context.Background(), nil, checkbox, []any{"selected"}); err != nil {
		t.Fatalf("checkbox array rejected: %v", err)
	}
	short := model.FormField{FieldType: "text"}
	if err := pf14cValidateFormValue(context.Background(), nil, short, string(make([]rune, 301))); err == nil {
		t.Fatal("short text above global limit was accepted")
	}
	long := model.FormField{FieldType: "text_long"}
	if err := pf14cValidateFormValue(context.Background(), nil, long, string(make([]rune, 10001))); err == nil {
		t.Fatal("long text above global limit was accepted")
	}
}

func TestRound8TicketTitleGlobalLimit(t *testing.T) {
	if err := validateTicketTitle(string(make([]rune, 301))); err == nil {
		t.Fatal("ticket title above 300 characters was accepted")
	}
	if err := validateTicketTitle(string(make([]rune, 300))); err != nil {
		t.Fatalf("ticket title at 300 characters was rejected: %v", err)
	}
}

func TestRound8DetailSectionRowLimit(t *testing.T) {
	if err := validateDetailRowPosition(49); err != nil {
		t.Fatalf("50th detail row rejected: %v", err)
	}
	if err := validateDetailRowPosition(50); err == nil {
		t.Fatal("51st detail row was accepted")
	}
}
