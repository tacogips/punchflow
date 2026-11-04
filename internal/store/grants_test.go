package store

import (
	"context"
	"reflect"
	"sort"
	"testing"
)

func seedGrantFixture(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		`INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES('u1','u1','u1@x.test','a','b','activated','en','normal','{}',1,1)`,
		`INSERT INTO folders(id,code,name,parent_id,editable,created_at,updated_at) VALUES('f-root','root','Root',NULL,0,1,1)`,
		`INSERT INTO folders(id,code,name,parent_id,editable,created_at,updated_at) VALUES('f-a','a','A','f-root',1,1,1)`,
		`INSERT INTO folders(id,code,name,parent_id,editable,created_at,updated_at) VALUES('f-a1','a1','A1','f-a',1,1,1)`,
		`INSERT INTO folders(id,code,name,parent_id,editable,created_at,updated_at) VALUES('f-b','b','B','f-root',1,1,1)`,
		`INSERT INTO organization_charts(id,name,current,created_at,updated_at) VALUES('c1','chart',1,1,1)`,
		`INSERT INTO teams(id,organization_chart_id,code,name,parent_id,created_at,updated_at) VALUES('t-top','c1','top','Top',NULL,1,1)`,
		`INSERT INTO teams(id,organization_chart_id,code,name,parent_id,created_at,updated_at) VALUES('t-mid','c1','mid','Mid','t-top',1,1)`,
		`INSERT INTO teams(id,organization_chart_id,code,name,parent_id,created_at,updated_at) VALUES('t-leaf','c1','leaf','Leaf','t-mid',1,1)`,
	}
	for _, statement := range statements {
		if _, err := db.Q().ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed %q: %v", statement, err)
		}
	}
}

func addRole(t *testing.T, db *DB, id, permissions string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Q().ExecContext(ctx, `INSERT INTO roles(id,name,editable,permission_list,created_at,updated_at) VALUES(?,?,1,?,1,1)`, id, id, permissions); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Q().ExecContext(ctx, `INSERT INTO role_memberships(role_id,user_id,created_at) VALUES(?,?,1)`, id, "u1"); err != nil {
		t.Fatal(err)
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestUserGrantsMergesRolesAndExpandsSubtrees(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedGrantFixture(t, db)
	addRole(t, db, "r1", `[{"permission":"ticket_read","restricted":true,"folderIds":["f-a"]},{"permission":"master","restricted":true,"generalMasterIds":["m1"]},{"permission":"team","restricted":true,"teamIds":["t-mid"]},{"permission":"user"},{"permission":"ticket_write","restricted":true}]`)
	addRole(t, db, "r2", `[{"permission":"ticket_read","restricted":true,"folderIds":["f-b"]},{"permission":"master","restricted":true,"generalMasterIds":["m2"]},{"permission":"master_read","restricted":false}]`)

	grants, err := UserGrants(ctx, db.Q(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	read := grants["ticket_read"]
	if read.Unrestricted || !reflect.DeepEqual(sortedKeys(read.Targets), []string{"f-a", "f-a1", "f-b"}) {
		t.Fatalf("ticket_read grant = %+v, want restricted to f-a, f-a1 and f-b (descendants included, roles unioned)", read)
	}
	if master := grants["master"]; master.Unrestricted || !reflect.DeepEqual(sortedKeys(master.Targets), []string{"m1", "m2"}) {
		t.Fatalf("master grant = %+v", master)
	}
	if team := grants["team"]; team.Unrestricted || !reflect.DeepEqual(sortedKeys(team.Targets), []string{"t-leaf", "t-mid"}) {
		t.Fatalf("team grant = %+v, want t-mid and its descendant t-leaf only", team)
	}
	if !grants["user"].Unrestricted || !grants["master_read"].Unrestricted {
		t.Fatalf("unrestricted entries lost: %+v", grants)
	}
	// ticket_write cannot be scoped in kickflow, so a restricted flag is ignored.
	if !grants["ticket_write"].Unrestricted {
		t.Fatalf("ticket_write grant = %+v, want unrestricted", grants["ticket_write"])
	}
	if !read.Allows("f-a1") || read.Allows("f-root") {
		t.Fatalf("Allows mismatch for %+v", read)
	}
}

func TestUserGrantsUnrestrictedEntryWinsOverRestricted(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedGrantFixture(t, db)
	addRole(t, db, "r1", `[{"permission":"workflow","restricted":true,"folderIds":["f-a"]}]`)
	addRole(t, db, "r2", `[{"permission":"workflow","restricted":false}]`)
	grants, err := UserGrants(ctx, db.Q(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !grants["workflow"].Unrestricted || !grants["workflow"].Allows("anything") {
		t.Fatalf("workflow grant = %+v, want unrestricted", grants["workflow"])
	}
}

func TestUserGrantsRestrictedWithoutTargetsCoversNothing(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedGrantFixture(t, db)
	addRole(t, db, "r1", `[{"permission":"team","restricted":true,"teamIds":[]}]`)
	grants, err := UserGrants(ctx, db.Q(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	team, held := grants["team"]
	if !held || team.Unrestricted || len(team.Targets) != 0 || team.Allows("t-top") {
		t.Fatalf("team grant = %+v (held=%v), want held but empty", team, held)
	}
}

func TestApplyDueOrgChartActivation(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedGrantFixture(t, db)
	if _, err := db.Q().ExecContext(ctx, `INSERT INTO organization_charts(id,name,current,created_at,updated_at) VALUES('c2','next',0,2,2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Q().ExecContext(ctx, `INSERT INTO organization_chart_activation_plans(id,organization_chart_id,due_on,created_at,updated_at) VALUES('p1','c2','2026-05-10',1,1)`); err != nil {
		t.Fatal(err)
	}
	now := Time(1_700_000_000_000)
	if id, err := ApplyDueOrgChartActivation(ctx, db, "2026-05-09", now); err != nil || id != "" {
		t.Fatalf("before the due date: id=%q err=%v, want nothing applied", id, err)
	}
	if id, err := ApplyDueOrgChartActivation(ctx, db, "2026-05-10", now); err != nil || id != "c2" {
		t.Fatalf("on the due date: id=%q err=%v, want c2", id, err)
	}
	if id, err := ApplyDueOrgChartActivation(ctx, db, "2026-05-11", now); err != nil || id != "" {
		t.Fatalf("second application: id=%q err=%v, want nothing (plan consumed)", id, err)
	}
	current, err := ActiveOrgChartID(ctx, db.Q())
	if err != nil || current != "c2" {
		t.Fatalf("current chart = %q err=%v, want c2", current, err)
	}
	if _, err := OrgChartActivationPlanByChart(ctx, db.Q(), "c2"); err != ErrNotFound {
		t.Fatalf("plan still present after application: err=%v", err)
	}
}
