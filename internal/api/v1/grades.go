package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

func (a *API) listGrades(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	sort := httpx.ParseSort(r, "level", "code")
	ids, total, err := store.GradeList(r.Context(), a.d.DB.Q(), page.Offset(), page.PerPage, sort.Field, sort.Desc)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	items, err := present.Grades(r.Context(), a.d.DB.Q(), ids)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (a *API) createGrade(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	var body struct {
		Name  string `json:"name"`
		Code  string `json:"code"`
		Level *int   `json:"level"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	if strings.TrimSpace(body.Name) == "" || body.Level == nil {
		kferr.Write(w, kferr.InvalidParameter("Missing name or level"))
		return
	}
	if len([]rune(body.Name)) > 300 || len([]rune(body.Code)) > 100 || *body.Level < 0 || *body.Level > 255 {
		kferr.Write(w, kferr.InvalidParameter("Invalid grade fields"))
		return
	}
	now := a.d.Clock.Now()
	id := store.NewID()
	if body.Code == "" {
		body.Code = strings.ReplaceAll(id, "-", "")[:16]
	}
	grade := model.Grade{ID: id, Code: body.Code, Name: body.Name, Level: *body.Level, CreatedAt: now, UpdatedAt: now}
	p, _ := auth.FromContext(r.Context())
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if err := store.GradeCreate(r.Context(), tx, grade); err != nil {
			return err
		}
		return pf18Audit(r, tx, p.User.ID, "grade.create", id, []string{"name", "code", "level"})
	})
	if err != nil {
		if pf18Unique(err) {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"level": {"has already been taken"}, "code": {"has already been taken"}}))
		} else {
			kferr.Write(w, err)
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present.Grade(grade))
}

func (a *API) getGrade(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "grades", r.PathValue("gradeId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	grade, err := store.GetGrade(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present.Grade(grade))
}

func (a *API) deleteGrade(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "grades", r.PathValue("gradeId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	p, _ := auth.FromContext(r.Context())
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		grade, err := store.GetGrade(r.Context(), tx, id)
		if err != nil {
			return kferr.ResourceNotFound()
		}
		if grade.IsDefault {
			return kferr.ValidationFailed(map[string][]string{"gradeId": {"cannot delete the default grade"}})
		}
		used, err := store.GradeInUse(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if used {
			return kferr.ValidationFailed(map[string][]string{"gradeId": {"is used by a membership"}})
		}
		if err := store.RemoveGradeDefinitionReferences(r.Context(), tx, grade.Code, a.d.Clock.Now()); err != nil {
			return err
		}
		if err := store.GradeDelete(r.Context(), tx, id); err != nil {
			return err
		}
		return pf18Audit(r, tx, p.User.ID, "grade.delete", id, []string{"id"})
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteEmpty(w)
}

func (a *API) updateGrade(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "grades", r.PathValue("gradeId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	var raw map[string]json.RawMessage
	if err := httpx.DecodeJSON(r, &raw); err != nil {
		kferr.Write(w, err)
		return
	}
	patch := store.GradePatch{}
	fields := []string{}
	if value, ok := raw["name"]; ok {
		var name string
		if json.Unmarshal(value, &name) != nil {
			kferr.Write(w, kferr.InvalidParameter("Invalid name"))
			return
		}
		if strings.TrimSpace(name) == "" {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"name": {"must not be blank"}}))
			return
		}
		if len([]rune(name)) > 300 {
			kferr.Write(w, kferr.InvalidParameter("Invalid name"))
			return
		}
		patch.Name = &name
		fields = append(fields, "name")
	}
	if value, ok := raw["code"]; ok {
		var code string
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &code) != nil || len([]rune(code)) > 100 {
			kferr.Write(w, kferr.InvalidParameter("Invalid code"))
			return
		}
		patch.Code = &code
		fields = append(fields, "code")
	}
	if value, ok := raw["level"]; ok {
		var level int
		if string(value) == "null" || json.Unmarshal(value, &level) != nil || level < 0 || level > 255 {
			kferr.Write(w, kferr.InvalidParameter("Invalid level"))
			return
		}
		patch.Level = &level
		fields = append(fields, "level")
	}
	if len(fields) == 0 {
		kferr.Write(w, kferr.InvalidParameter("Empty update"))
		return
	}
	p, _ := auth.FromContext(r.Context())
	now := a.d.Clock.Now()
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if patch.Code != nil {
			current, err := store.GetGrade(r.Context(), tx, id)
			if err != nil {
				return err
			}
			if err := store.ReconcileDefinitionCodeChange(r.Context(), tx, "grade", current.Code, *patch.Code, now); err != nil {
				return err
			}
		}
		if err := store.GradeUpdate(r.Context(), tx, id, patch, store.Millis(now)); err != nil {
			return err
		}
		return pf18Audit(r, tx, p.User.ID, "grade.update", id, fields)
	})
	if err != nil {
		if pf18Unique(err) {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"level": {"has already been taken"}, "code": {"has already been taken"}}))
		} else {
			kferr.Write(w, err)
		}
		return
	}
	grade, err := store.GetGrade(r.Context(), a.d.DB.Q(), id)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present.Grade(grade))
}

func (a *API) setDefaultGrade(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "grades", r.PathValue("gradeId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	p, _ := auth.FromContext(r.Context())
	now := a.d.Clock.Now()
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if err := store.GradeSetDefault(r.Context(), tx, id, store.Millis(now)); err != nil {
			return kferr.ResourceNotFound()
		}
		return pf18Audit(r, tx, p.User.ID, "grade.setDefault", id, []string{"isDefault"})
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteEmpty(w)
}
