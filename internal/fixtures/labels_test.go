package fixtures

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func labelsFixtureSet(t *testing.T) *Set {
	t.Helper()
	set, err := Parse(pf06MinimalFS())
	if err != nil {
		t.Fatal(err)
	}
	notes := "waiting for the vendor"
	set.Labels = []LabelFixture{
		{ID: "00000000-0000-4000-8000-0000000000a1", Name: "Urgent", Color: "ff0000"},
		{ID: "00000000-0000-4000-8000-0000000000a2", Name: "Vendor", Description: &notes, Color: "00ff00"},
	}
	set.SubStatuses = []SubStatusFixture{
		{ID: "00000000-0000-4000-8000-0000000000b1", Code: "vendor-wait", Name: "Vendor wait", Notes: &notes, AllowRejection: true},
	}
	code := "vendor-wait"
	set.Routes[0].Steps[0].SubStatus = &code
	set.Workflows[0].TicketNumberingTiming = "sub_status_attached"
	set.Workflows[0].TicketNumberingSubStatus = &code
	return set
}

func TestValidateLabelsAndSubStatuses(t *testing.T) {
	if err := Validate(labelsFixtureSet(t)); err != nil {
		t.Fatalf("Validate(valid labels): %v", err)
	}
	str := func(s string) *string { return &s }
	cases := []struct {
		name   string
		mutate func(*Set)
		file   string
		path   string
	}{
		{"duplicate label name", func(s *Set) { s.Labels[1].Name = "Urgent" }, FileLabels, "[1].name"},
		{"blank label name", func(s *Set) { s.Labels[0].Name = " " }, FileLabels, "[0].name"},
		{"bad label color", func(s *Set) { s.Labels[0].Color = "#ff0000" }, FileLabels, "[0].color"},
		{"uppercase label color", func(s *Set) { s.Labels[0].Color = "FF0000" }, FileLabels, "[0].color"},
		{"duplicate label id", func(s *Set) { s.Labels[1].ID = s.Labels[0].ID }, FileLabels, "$[1].id"},
		{"duplicate sub-status code", func(s *Set) {
			s.SubStatuses = append(s.SubStatuses, SubStatusFixture{ID: "00000000-0000-4000-8000-0000000000b2", Code: "vendor-wait", Name: "Again"})
		}, FileSubStatuses, "[1].code"},
		{"blank sub-status code", func(s *Set) {
			s.SubStatuses[0].Code = ""
			s.Routes[0].Steps[0].SubStatus, s.Workflows[0].TicketNumberingSubStatus = nil, nil
		}, FileSubStatuses, "[0].code"},
		{"blank sub-status name", func(s *Set) { s.SubStatuses[0].Name = "" }, FileSubStatuses, "[0].name"},
		{"unknown route sub-status", func(s *Set) { s.Routes[0].Steps[0].SubStatus = str("missing") }, FileRoutes, "[0].steps[0].subStatus"},
		{"unknown numbering sub-status", func(s *Set) { s.Workflows[0].TicketNumberingSubStatus = str("missing") }, FileWorkflows, "[0].ticketNumberingSubStatus"},
		{"numbering sub-status with another timing", func(s *Set) { s.Workflows[0].TicketNumberingTiming = "opened" }, FileWorkflows, "[0].ticketNumberingSubStatus"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := labelsFixtureSet(t)
			tc.mutate(set)
			var validationErr *ValidationError
			if err := Validate(set); !errors.As(err, &validationErr) {
				t.Fatalf("Validate error=%v; want ValidationError", err)
			}
			if len(validationErr.Problems) != 1 {
				t.Fatalf("problems=%+v; want exactly one", validationErr.Problems)
			}
			if got := validationErr.Problems[0]; got.File != tc.file || got.Path != tc.path {
				t.Fatalf("problem=%+v; want %s %s", got, tc.file, tc.path)
			}
		})
	}
}

func TestLabelsAndSubStatusesRoundTrip(t *testing.T) {
	ctx := context.Background()
	set := labelsFixtureSet(t)
	files, err := pf06EncodeSet(set)
	if err != nil {
		t.Fatal(err)
	}
	db := pf06OpenDB(t)
	summary, err := Load(ctx, db, pf06MapFS(files), pf06Clock())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if summary.Counts["labels"] != 2 || summary.Counts["sub_statuses"] != 1 {
		t.Fatalf("counts=%v, want 2 labels and 1 sub-status", summary.Counts)
	}
	exported, err := Export(ctx, db.Q())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{FileLabels, FileSubStatuses, FileRoutes, FileWorkflows} {
		if !bytes.Equal(exported[name], files[name]) {
			t.Errorf("export %s differs from the loaded fixture:\n%s\nvs\n%s", name, exported[name], files[name])
		}
	}
	// A reset replaces the definitions with the fixture set again.
	if _, err = Reset(ctx, db, pf06MapFS(files), "", pf06Clock()); err != nil {
		t.Fatalf("reset: %v", err)
	}
	var labels, statuses int
	if err = db.Q().QueryRowContext(ctx, `SELECT (SELECT count(*) FROM labels),(SELECT count(*) FROM sub_statuses)`).Scan(&labels, &statuses); err != nil {
		t.Fatal(err)
	}
	if labels != 2 || statuses != 1 {
		t.Fatalf("after reset labels=%d sub_statuses=%d", labels, statuses)
	}
}

func TestParseStrictLabelFiles(t *testing.T) {
	for _, name := range []string{FileLabels, FileSubStatuses} {
		files := map[string][]byte{}
		base, err := pf06EncodeSet(labelsFixtureSet(t))
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range base {
			files[k] = v
		}
		files[name] = []byte(`[{"unexpected": true}]`)
		_, err = Parse(pf06MapFS(files))
		var validationErr *ValidationError
		if !errors.As(err, &validationErr) || validationErr.Problems[0].File != name {
			t.Fatalf("Parse(%s with unknown key) error=%v, want ValidationError for the file", name, err)
		}
	}
}
