package store

import (
	"testing"
	"time"
)

func TestInvalidateRoutesAndDependentWorkflowsForDeletedPrincipals(t *testing.T) {
	ctx, db := openMemory(t)
	for _, statement := range []string{
		`INSERT INTO folders(id,code,name,created_at,updated_at) VALUES('f1','folder','Folder',1,1)`,
		`INSERT INTO routes(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('r-user','route-user','visible','f1','{"steps":[{"userCodes":["retired-user"]}]}',1,1)`,
		`INSERT INTO routes(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('r-team','route-team','visible','f1','{"steps":[{"targets":[{"teamCode":"retired-team"}]}]}',1,1)`,
		`INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('w-user','workflow-user','visible','f1','{"routeCode":"route-user"}',1,1)`,
		`INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('w-team','workflow-team','visible','f1','{"routeCode":"route-team"}',1,1)`,
	} {
		if _, err := db.Q().ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := InvalidateRoutesReferencing(ctx, db.Q(), "retired-user", true, time.UnixMilli(100)); err != nil {
		t.Fatal(err)
	}
	if err := InvalidateRoutesReferencing(ctx, db.Q(), "retired-team", false, time.UnixMilli(200)); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"routes", "workflows"} {
		rows, err := db.Q().QueryContext(ctx, `SELECT status FROM `+table+` ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var status string
			if err := rows.Scan(&status); err != nil {
				t.Fatal(err)
			}
			want := "error"
			if table == "workflows" {
				want = "invisible"
			}
			if status != want {
				t.Errorf("%s status=%q, want %q", table, status, want)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		_ = rows.Close()
	}
}

func TestRemoveDeletedTeamFromRolePermissionTargets(t *testing.T) {
	ctx, db := openMemory(t)
	if _, err := db.Q().ExecContext(ctx, `INSERT INTO roles(id,name,editable,permission_list,created_at,updated_at) VALUES('role','Role',1,'[{"permission":"team","restricted":true,"teamIds":["deleted-team","retained-team"]}]',1,1)`); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTeamFromRoleTargets(ctx, db.Q(), "deleted-team", time.UnixMilli(100)); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := db.Q().QueryRowContext(ctx, `SELECT permission_list FROM roles WHERE id='role'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != `[{"permission":"team","restricted":true,"teamIds":["retained-team"]}]` {
		t.Fatalf("permission list=%s", raw)
	}
}
