package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing/fstest"

	"punchflow/internal/auth"
	"punchflow/internal/fixtures"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

func (a *API) createWorkflow(w http.ResponseWriter, r *http.Request) {
	p, ok := pf24Principal(w, r, model.PermissionWorkflow)
	if !ok {
		return
	}
	var raw json.RawMessage
	if err := httpx.DecodeJSON(r, &raw); err != nil {
		kferr.Write(w, err)
		return
	}
	if len(raw) == 0 {
		kferr.Write(w, kferr.InvalidJSON())
		return
	}
	var def model.WorkflowDef
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&def); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			kferr.Write(w, kferr.InvalidParameter(strings.TrimPrefix(err.Error(), "json: ")))
		} else {
			var typeErr *json.UnmarshalTypeError
			if errors.As(err, &typeErr) {
				kferr.Write(w, kferr.InvalidParameter(typeErr.Error()))
			} else {
				kferr.Write(w, kferr.InvalidJSON())
			}
		}
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		kferr.Write(w, kferr.InvalidJSON())
		return
	}
	if def.ID == "" {
		def.ID = store.NewID()
	}
	ctx := r.Context()
	err := a.d.DB.WithTx(ctx, func(tx *store.Tx) error { return a.insertWorkflow(ctx, tx, p, def) })
	if err != nil {
		kferr.Write(w, err)
		return
	}
	result, err := present.WorkflowDetail(ctx, a.d.DB.Q(), def.ID)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *API) insertWorkflow(ctx context.Context, tx *store.Tx, p *auth.Principal, def model.WorkflowDef) error {
	var id, code string
	err := tx.QueryRowContext(ctx, "SELECT id, code FROM workflows WHERE id=? OR code=? LIMIT 1", def.ID, def.Code).Scan(&id, &code)
	if err == nil {
		if id == def.ID {
			return workflowConflict("id")
		}
		return workflowConflict("code")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := validateNewWorkflow(ctx, tx, def); err != nil {
		return err
	}
	folderID, err := store.ResolveID(ctx, tx, "folders", def.FolderCode)
	if err != nil {
		return err
	}
	if !p.AllowsAny(folderID, model.PermissionWorkflow) {
		return kferr.MissingPermission("Missing permission")
	}
	raw, err := json.Marshal(def)
	if err != nil {
		return err
	}
	now := store.Millis(a.d.Clock.Now())
	_, err = tx.ExecContext(ctx, "INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", def.ID, def.Code, def.Status, folderID, string(raw), now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			if strings.Contains(err.Error(), "workflows.code") {
				return workflowConflict("code")
			}
			return workflowConflict("id")
		}
		return err
	}
	return pf24Audit(ctx, tx, p, "workflow.create", map[string]string{"workflowId": def.ID})
}

func workflowConflict(field string) error {
	return kferr.New(http.StatusConflict, "conflict", "workflow "+field+" already exists")
}

func validateNewWorkflow(ctx context.Context, tx *store.Tx, def model.WorkflowDef) error {
	files, err := fixtures.Export(ctx, tx)
	if err != nil {
		return err
	}
	fsys := fstest.MapFS{}
	for name, data := range files {
		fsys[name] = &fstest.MapFile{Data: data}
	}
	set, err := fixtures.Parse(fsys)
	if err != nil {
		return err
	}
	baseline, err := workflowValidationProblems(fixtures.Validate(set))
	if err != nil {
		return err
	}
	set.Workflows = append(set.Workflows, def)
	candidate, err := workflowValidationProblems(fixtures.Validate(set))
	if err != nil {
		return err
	}
	counts := make(map[fixtures.Problem]int, len(baseline))
	for _, problem := range baseline {
		counts[problem]++
	}
	var added []string
	for _, problem := range candidate {
		if counts[problem] > 0 {
			counts[problem]--
			continue
		}
		added = append(added, fmt.Sprintf("%s: %s: %s", problem.File, problem.Path, problem.Message))
	}
	if len(added) > 0 {
		return kferr.InvalidParameter(strings.Join(added, "; "))
	}
	return nil
}

func workflowValidationProblems(err error) ([]fixtures.Problem, error) {
	if err == nil {
		return nil, nil
	}
	var validationErr *fixtures.ValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Problems, nil
	}
	return nil, err
}
