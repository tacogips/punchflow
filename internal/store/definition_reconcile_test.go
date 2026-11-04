package store

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"punchflow/internal/model"
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

func TestDeletionReconciliationHandlesConditionalRoutesAndNominationTargets(t *testing.T) {
	ctx, db := openMemory(t)
	for _, statement := range []string{
		`INSERT INTO folders(id,code,name,created_at,updated_at) VALUES('f1','folder','Folder',1,1)`,
		`INSERT INTO routes(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('fixed','fixed-route','visible','f1','{"steps":[{"stepType":"team","targets":[{"teamCode":"gone"}]}]}',1,1)`,
		`INSERT INTO routes(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('nom','nom-route','visible','f1','{"steps":[{"stepType":"author_customizable","targets":[{"teamCode":"gone"},{"teamCode":"keep"}]}]}',1,1)`,
		`INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('conditional','conditional','visible','f1','{"routeCode":"safe","routeConditions":[{"routeCode":"fixed-route"}]}',1,1)`,
	} {
		if _, err := db.Q().ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := InvalidateRoutesReferencing(ctx, db.Q(), "gone", false, time.UnixMilli(100)); err != nil {
		t.Fatal(err)
	}
	var fixedStatus, nomStatus, workflowStatus string
	_ = db.Q().QueryRowContext(ctx, `SELECT status FROM routes WHERE id='fixed'`).Scan(&fixedStatus)
	_ = db.Q().QueryRowContext(ctx, `SELECT status FROM routes WHERE id='nom'`).Scan(&nomStatus)
	_ = db.Q().QueryRowContext(ctx, `SELECT status FROM workflows WHERE id='conditional'`).Scan(&workflowStatus)
	if fixedStatus != model.RouteStatusError || nomStatus != model.RouteStatusVisible || workflowStatus != model.WorkflowStatusInvisible {
		t.Fatalf("statuses fixed=%s nomination=%s workflow=%s", fixedStatus, nomStatus, workflowStatus)
	}
	var definition string
	if err := db.Q().QueryRowContext(ctx, `SELECT definition_json FROM routes WHERE id='nom'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(definition, "gone") || !strings.Contains(definition, "keep") {
		t.Fatalf("nomination targets after cleanup: %s", definition)
	}
}

func TestDefinitionCodeRenameAndGradeCleanup(t *testing.T) {
	ctx, db := openMemory(t)
	if _, err := db.Q().ExecContext(ctx, `INSERT INTO folders(id,code,name,created_at,updated_at) VALUES('f1','folder','Folder',1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Q().ExecContext(ctx, `INSERT INTO routes(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('r','route','visible','f1','{"steps":[{"userCodes":["old-user"],"targets":[{"teamCode":"old-team","gradeSymbol":"greater_than_or_equal","gradeCodes":["old-grade"]}]}]}',1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Q().ExecContext(ctx, `INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('w','workflow','visible','f1','{"availableTeams":[{"teamCode":"old-team","gradeCodes":["old-grade"]}],"routeConditions":[{"routeCode":"r","conditionFields":[{"generalMasterCode":"old-master","generalMasterItemCode":"old-item"}]}]}',1,1)`); err != nil {
		t.Fatal(err)
	}
	for _, rename := range []struct{ kind, old, new string }{{"user", "old-user", "new-user"}, {"team", "old-team", "new-team"}, {"grade", "old-grade", "new-grade"}, {"master", "old-master", "new-master"}, {"master_item", "old-item", "new-item"}} {
		if err := ReconcileDefinitionCodeChange(ctx, db.Q(), rename.kind, rename.old, rename.new, time.UnixMilli(100)); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveGradeDefinitionReferences(ctx, db.Q(), "new-grade", time.UnixMilli(200)); err != nil {
		t.Fatal(err)
	}
	var route, workflow string
	if err := db.Q().QueryRowContext(ctx, `SELECT definition_json FROM routes WHERE id='r'`).Scan(&route); err != nil {
		t.Fatal(err)
	}
	if err := db.Q().QueryRowContext(ctx, `SELECT definition_json FROM workflows WHERE id='w'`).Scan(&workflow); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(route, "new-user") || !strings.Contains(route, "new-team") || strings.Contains(route, "new-grade") || strings.Contains(route, "gradeSymbol") {
		t.Fatalf("route references = %s", route)
	}
	if !strings.Contains(workflow, "new-team") || strings.Contains(workflow, "new-grade") || !strings.Contains(workflow, "new-master") || !strings.Contains(workflow, "new-item") {
		t.Fatalf("workflow references = %s", workflow)
	}
}

func TestConditionReferencesReconcileByTypeAndMaster(t *testing.T) {
	ctx, db := openMemory(t)
	for _, statement := range []string{
		`INSERT INTO folders(id,code,name,created_at,updated_at) VALUES('f1','folder','Folder',1,1)`,
		`INSERT INTO routes(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('r','route','visible','f1','{"steps":[{"routeStepCondition":{"routeStepConditionFields":[{"fieldKey":"author_team","value":"old-team"},{"fieldKey":"author_grade","value":"old-grade"},{"fieldKey":"text_variable","value":"old-team"}]}}]}',1,1)`,
		`INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('w','workflow','visible','f1','{"routeConditions":[{"conditionFields":[{"fieldKey":"author_team","value":"old-team"},{"fieldKey":"author_grade","value":"old-grade"}]}],"sections":[{"slipFields":[{"generalMasterCode":"master-a","defaultGeneralMasterItemCode":"shared"},{"generalMasterCode":"master-b","defaultGeneralMasterItemCode":"shared"}]}]}',1,1)`,
	} {
		if _, err := db.Q().ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := ReconcileDefinitionCodeChange(ctx, db.Q(), "team", "old-team", "new-team", time.UnixMilli(10)); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileDefinitionCodeChange(ctx, db.Q(), "grade", "old-grade", "new-grade", time.UnixMilli(10)); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileDefinitionCodeChange(ctx, db.Q(), "master_item", "shared", "renamed", time.UnixMilli(10), "master-a"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTeamDefinitionReferences(ctx, db.Q(), "new-team", time.UnixMilli(20)); err != nil {
		t.Fatal(err)
	}
	if err := RemoveGradeDefinitionReferences(ctx, db.Q(), "new-grade", time.UnixMilli(20)); err != nil {
		t.Fatal(err)
	}
	var route, workflow string
	if err := db.Q().QueryRowContext(ctx, `SELECT definition_json FROM routes WHERE id='r'`).Scan(&route); err != nil {
		t.Fatal(err)
	}
	if err := db.Q().QueryRowContext(ctx, `SELECT definition_json FROM workflows WHERE id='w'`).Scan(&workflow); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(route, `"fieldKey":"author_team"`) || strings.Contains(route, `"fieldKey":"author_grade"`) || !strings.Contains(route, `"fieldKey":"text_variable"`) || !strings.Contains(route, `"value":"old-team"`) {
		t.Fatalf("route conditions after reconciliation: %s", route)
	}
	if strings.Contains(workflow, `"fieldKey":"author_team"`) || strings.Contains(workflow, `"fieldKey":"author_grade"`) {
		t.Fatalf("workflow conditions after reconciliation: %s", workflow)
	}
	if !strings.Contains(workflow, `"generalMasterCode":"master-a"`) || !strings.Contains(workflow, `"defaultGeneralMasterItemCode":"renamed"`) || !strings.Contains(workflow, `"generalMasterCode":"master-b"`) || !strings.Contains(workflow, `"defaultGeneralMasterItemCode":"shared"`) {
		t.Fatalf("master scoped defaults after reconciliation: %s", workflow)
	}
}

func TestRemoveScopedMasterAndFolderTargets(t *testing.T) {
	ctx, db := openMemory(t)
	if _, err := db.Q().ExecContext(ctx, `INSERT INTO roles(id,name,editable,permission_list,created_at,updated_at) VALUES('role','Role',1,'[{"permission":"master","generalMasterIds":["master","keep"]},{"permission":"workflow","folderIds":["folder","keep-folder"]}]',1,1)`); err != nil {
		t.Fatal(err)
	}
	if err := RemoveGeneralMasterFromRoleTargets(ctx, db.Q(), "master", time.UnixMilli(100)); err != nil {
		t.Fatal(err)
	}
	if err := RemoveFoldersFromRoleTargets(ctx, db.Q(), []string{"folder"}, time.UnixMilli(100)); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := db.Q().QueryRowContext(ctx, `SELECT permission_list FROM roles WHERE id='role'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var permissions []struct {
		GeneralMasterIDs []string `json:"generalMasterIds"`
		FolderIDs        []string `json:"folderIds"`
	}
	if err := json.Unmarshal([]byte(raw), &permissions); err != nil {
		t.Fatal(err)
	}
	if len(permissions) != 2 || len(permissions[0].GeneralMasterIDs) != 1 || permissions[0].GeneralMasterIDs[0] != "keep" || len(permissions[1].FolderIDs) != 1 || permissions[1].FolderIDs[0] != "keep-folder" {
		t.Fatalf("role scopes=%s", raw)
	}
}
