package fixtures

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing"
)

// Two charts may reuse team codes; each chart's teams, parents and memberships must stay chart-local.
func TestLoadOrgChartsWithSharedTeamCodes(t *testing.T) {
	ctx := context.Background()
	base, err := fs.ReadFile(pf06MinimalFS(), "organization-charts.json")
	if err != nil {
		t.Fatal(err)
	}
	var charts []OrgChartFixture
	if err := json.Unmarshal(base, &charts); err != nil {
		t.Fatal(err)
	}
	next := charts[0]
	next.ID = "00000000-0000-4000-8000-0000000000a7"
	next.Name = "Next"
	next.Current = false
	next.Teams = append([]TeamFixture(nil), charts[0].Teams...)
	next.Teams[0].ID = "00000000-0000-4000-8000-0000000000a8"
	next.Teams[1].ID = "00000000-0000-4000-8000-0000000000a9"
	charts = append(charts, next)
	raw, err := json.Marshal(charts)
	if err != nil {
		t.Fatal(err)
	}

	files := map[string][]byte{}
	entries, err := fs.ReadDir(pf06MinimalFS(), ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := fs.ReadFile(pf06MinimalFS(), e.Name())
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = data
	}
	files["organization-charts.json"] = raw

	db := pf06OpenDB(t)
	if _, err := Load(ctx, db, pf06MapFS(files), pf06Clock()); err != nil {
		t.Fatalf("load charts sharing team codes: %v", err)
	}

	wantParent := map[string]string{
		"00000000-0000-4000-8000-000000000009": "00000000-0000-4000-8000-000000000008",
		"00000000-0000-4000-8000-0000000000a9": "00000000-0000-4000-8000-0000000000a8",
	}
	for child, parent := range wantParent {
		var got string
		if err := db.Q().QueryRowContext(ctx, "SELECT parent_id FROM teams WHERE id=?", child).Scan(&got); err != nil {
			t.Fatalf("parent of %s: %v", child, err)
		}
		if got != parent {
			t.Errorf("parent of %s = %s; want %s", child, got, parent)
		}
	}
	for _, team := range []string{
		"00000000-0000-4000-8000-000000000008", "00000000-0000-4000-8000-000000000009",
		"00000000-0000-4000-8000-0000000000a8", "00000000-0000-4000-8000-0000000000a9",
	} {
		var n int
		if err := db.Q().QueryRowContext(ctx, "SELECT COUNT(*) FROM team_memberships WHERE team_id=?", team).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("team %s has %d memberships; want 1", team, n)
		}
	}
}
