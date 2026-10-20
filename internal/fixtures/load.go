package fixtures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// Summary reports inserted row counts by SQLite table.
type Summary struct{ Counts map[string]int }

// Load parses and validates fixtures, then inserts them into an empty database.
func Load(ctx context.Context, db *store.DB, fsys fs.FS, clock kftime.Clock) (Summary, error) {
	s, err := Parse(fsys)
	if err != nil {
		return Summary{}, err
	}
	if err := Validate(s); err != nil {
		return Summary{}, err
	}
	empty, err := db.IsEmpty(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("fixtures: check database emptiness: %w", err)
	}
	if !empty {
		return Summary{}, fmt.Errorf("database is not empty")
	}
	return pf06Insert(ctx, db, s, clock)
}

// Reset replaces all application fixture data atomically and clears uploaded files.
func Reset(ctx context.Context, db *store.DB, fsys fs.FS, filesDir string, clock kftime.Clock) (Summary, error) {
	s, err := Parse(fsys)
	if err != nil {
		return Summary{}, err
	}
	if err := Validate(s); err != nil {
		return Summary{}, err
	}
	now := clock.Now()
	counts := Summary{Counts: map[string]int{}}
	if err := db.WithTx(ctx, func(tx *store.Tx) error {
		if err := store.TruncateAll(ctx, tx); err != nil {
			return err
		}
		var err error
		counts, err = pf06InsertTx(ctx, tx, s, now)
		return err
	}); err != nil {
		return Summary{}, err
	}
	if filesDir != "" {
		if err := pf06ClearFiles(filesDir); err != nil {
			return Summary{}, fmt.Errorf("fixtures: clear files directory: %w", err)
		}
	}
	return counts, nil
}

func pf06Insert(ctx context.Context, db *store.DB, s *Set, clock kftime.Clock) (Summary, error) {
	now := clock.Now()
	var out Summary
	err := db.WithTx(ctx, func(tx *store.Tx) error { var err error; out, err = pf06InsertTx(ctx, tx, s, now); return err })
	if err != nil {
		return Summary{}, err
	}
	return out, nil
}

func pf06InsertTx(ctx context.Context, q store.Querier, s *Set, now time.Time) (Summary, error) {
	counts := Summary{Counts: map[string]int{}}
	ms := store.Millis(now)
	insert := func(table, query string, args ...any) error {
		if _, err := q.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("fixtures: insert %s: %w", table, err)
		}
		counts.Counts[table]++
		return nil
	}
	jsonValue := func(v any) (string, error) { b, err := json.Marshal(v); return string(b), err }
	t := s.Tenant.Tenant
	allow, err := jsonValue(t.IPAllowlist)
	if err != nil {
		return counts, err
	}
	domains, err := jsonValue(t.AllowedEmailDomains)
	if err != nil {
		return counts, err
	}
	if err = insert("tenant", "INSERT INTO tenant(id,public_id,name,plan,mode,ip_allowlist,allowed_email_domains,self_service_proxy,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)", t.ID, t.PublicID, t.Name, t.Plan, t.Mode, allow, domains, t.SelfServiceProxy, ms, ms); err != nil {
		return counts, err
	}
	for _, g := range s.Grades {
		if err = insert("grades", "INSERT INTO grades(id,code,name,level,is_default,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", g.ID, g.Code, g.Name, g.Level, g.IsDefault, ms, ms); err != nil {
			return counts, err
		}
	}
	userIDs := map[string]string{}
	for _, u := range s.Users {
		custom, e := jsonValue(u.CustomFields)
		if e != nil {
			return counts, e
		}
		if err = insert("users", "INSERT INTO users(id,code,email,first_name,last_name,employee_id,status,locale,user_type,custom_fields,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", u.ID, u.Code, u.Email, u.FirstName, u.LastName, u.EmployeeID, u.Status, u.Locale, u.UserType, custom, ms, ms); err != nil {
			return counts, err
		}
		userIDs[u.Code] = u.ID
		if u.Status == model.UserStatusInvited {
			inv := store.NewID()
			expiry := store.Millis(now.Add(7 * 24 * time.Hour))
			if err = insert("user_invitations", "INSERT INTO user_invitations(id,user_id,user_email,user_locale,expires_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", inv, u.ID, u.Email, u.Locale, expiry, ms, ms); err != nil {
				return counts, err
			}
		}
	}
	gradeIDs := map[string]string{}
	for _, g := range s.Grades {
		gradeIDs[g.Code] = g.ID
	}
	teamIDs := map[string]string{}
	for _, chart := range s.OrgCharts {
		if err = insert("organization_charts", "INSERT INTO organization_charts(id,name,current,created_at,updated_at) VALUES(?,?,?,?,?)", chart.ID, chart.Name, chart.Current, ms, ms); err != nil {
			return counts, err
		}
		teams := make(map[string]TeamFixture, len(chart.Teams))
		for _, team := range chart.Teams {
			teams[team.Code] = team
		}
		inserted := map[string]bool{}
		var insertTeam func(string) error
		insertTeam = func(code string) error {
			if inserted[code] {
				return nil
			}
			team := teams[code]
			if team.ParentCode != nil {
				if err := insertTeam(*team.ParentCode); err != nil {
					return err
				}
			}
			var parent any
			if team.ParentCode != nil {
				parent = teamIDs[*team.ParentCode]
			}
			if err := insert("teams", "INSERT INTO teams(id,organization_chart_id,code,name,notes,parent_id,approve_only,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)", team.ID, chart.ID, team.Code, team.Name, team.Notes, parent, team.ApproveOnly, ms, ms); err != nil {
				return err
			}
			teamIDs[code] = team.ID
			inserted[code] = true
			return nil
		}
		for _, team := range chart.Teams {
			if err := insertTeam(team.Code); err != nil {
				return counts, err
			}
		}
	}
	for _, chart := range s.OrgCharts {
		for _, team := range chart.Teams {
			for _, m := range team.Memberships {
				grades := make([]string, 0, len(m.GradeCodes))
				for _, c := range m.GradeCodes {
					grades = append(grades, gradeIDs[c])
				}
				raw, e := jsonValue(grades)
				if e != nil {
					return counts, e
				}
				if err = insert("team_memberships", "INSERT INTO team_memberships(id,team_id,user_id,leader,grade_ids,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", store.NewID(), teamIDs[team.Code], userIDs[m.UserCode], m.Leader, raw, ms, ms); err != nil {
					return counts, err
				}
			}
		}
	}
	for _, r := range s.Roles {
		raw, e := jsonValue(r.PermissionList)
		if e != nil {
			return counts, e
		}
		if err = insert("roles", "INSERT INTO roles(id,name,editable,permission_list,created_at,updated_at) VALUES(?,?,?,?,?,?)", r.ID, r.Name, r.Editable, raw, ms, ms); err != nil {
			return counts, err
		}
		for _, code := range r.MemberCodes {
			if err = insert("role_memberships", "INSERT INTO role_memberships(role_id,user_id,created_at) VALUES(?,?,?)", r.ID, userIDs[code], ms); err != nil {
				return counts, err
			}
		}
	}
	for _, c := range s.Categories {
		if err = insert("categories", "INSERT INTO categories(id,code,name,created_at,updated_at) VALUES(?,?,?,?,?)", c.ID, c.Code, c.Name, ms, ms); err != nil {
			return counts, err
		}
	}
	folderIDs := map[string]string{}
	folders := make(map[string]FolderFixture, len(s.Folders))
	for _, f := range s.Folders {
		folders[f.Code] = f
	}
	insertedFolders := map[string]bool{}
	var insertFolder func(string) error
	insertFolder = func(code string) error {
		if insertedFolders[code] {
			return nil
		}
		f := folders[code]
		if f.ParentCode != nil {
			if err := insertFolder(*f.ParentCode); err != nil {
				return err
			}
		}
		var parent any
		if f.ParentCode != nil {
			parent = folderIDs[*f.ParentCode]
		}
		if err := insert("folders", "INSERT INTO folders(id,code,name,description,parent_id,editable,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", f.ID, f.Code, f.Name, f.Description, parent, f.Editable, ms, ms); err != nil {
			return err
		}
		folderIDs[code] = f.ID
		insertedFolders[code] = true
		return nil
	}
	for _, f := range s.Folders {
		if err := insertFolder(f.Code); err != nil {
			return counts, err
		}
	}
	for _, m := range s.GeneralMasters {
		if err = insert("general_masters", "INSERT INTO general_masters(id,code,name,description,default_sort_by,initial_display_code,initial_display_created_at,initial_display_description,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)", m.ID, m.Code, m.Name, m.Description, m.DefaultSortBy, m.InitialDisplayCode, m.InitialDisplayCreatedAt, m.InitialDisplayDescription, ms, ms); err != nil {
			return counts, err
		}
		fieldIDs := map[string]string{}
		for _, f := range m.Fields {
			opts, e := jsonValue(f.Options)
			if e != nil {
				return counts, e
			}
			if err = insert("general_master_fields", "INSERT INTO general_master_fields(id,general_master_id,code,title,description,field_type,required,field_order,visible,initial_display,options,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", f.ID, m.ID, f.Code, f.Title, f.Description, f.FieldType, f.Required, f.FieldOrder, f.Visible, f.InitialDisplay, opts, ms, ms); err != nil {
				return counts, err
			}
			fieldIDs[f.Code] = f.ID
		}
		for _, it := range m.Items {
			inputs := map[string]any{}
			inputIDs := map[string]string{}
			for _, in := range it.Inputs {
				fieldID := fieldIDs[in.FieldCode]
				inputs[fieldID] = in.Value
				inputIDs[fieldID] = in.ID
			}
			raw, e := jsonValue(inputs)
			if e != nil {
				return counts, e
			}
			inputIDsRaw, e := jsonValue(inputIDs)
			if e != nil {
				return counts, e
			}
			if err = insert("general_master_items", "INSERT INTO general_master_items(id,general_master_id,code,name,description,starts_on,ends_on,inputs,input_ids,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)", it.ID, m.ID, it.Code, it.Name, it.Description, it.StartsOn, it.EndsOn, raw, inputIDsRaw, ms, ms); err != nil {
				return counts, err
			}
		}
	}
	for _, r := range s.Routes {
		raw, e := jsonValue(r)
		if e != nil {
			return counts, e
		}
		if err = insert("routes", "INSERT INTO routes(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", r.ID, r.Code, r.Status, folderIDs[r.FolderCode], raw, ms, ms); err != nil {
			return counts, err
		}
	}
	workflowIDs := map[string]string{}
	for _, w := range s.Workflows {
		raw, e := jsonValue(w)
		if e != nil {
			return counts, e
		}
		if err = insert("workflows", "INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", w.ID, w.Code, w.Status, folderIDs[w.FolderCode], raw, ms, ms); err != nil {
			return counts, err
		}
		workflowIDs[w.Code] = w.ID
	}
	for _, x := range s.ProxyApplicants {
		if err = pf06InsertProxy(insert, "proxy_applicants", x, userIDs, workflowIDs, ms); err != nil {
			return counts, err
		}
	}
	for _, x := range s.ProxyApprovers {
		if err = pf06InsertProxy(insert, "proxy_approvers", x, userIDs, workflowIDs, ms); err != nil {
			return counts, err
		}
	}
	for _, x := range s.Tokens {
		sum := sha256.Sum256([]byte(x.Token))
		var user any
		if x.UserCode != "" {
			user = userIDs[x.UserCode]
		}
		if err = insert("api_tokens", "INSERT INTO api_tokens(id,kind,name,description,token_hash,user_id,fixture_plaintext,created_at) VALUES(?,?,?,?,?,?,?,?)", x.ID, x.Kind, x.Name, x.Description, hex.EncodeToString(sum[:]), user, x.Token, ms); err != nil {
			return counts, err
		}
	}
	for _, x := range s.Webhooks {
		wf := make([]string, 0, len(x.WorkflowCodes))
		for _, c := range x.WorkflowCodes {
			wf = append(wf, workflowIDs[c])
		}
		wfraw, e := jsonValue(wf)
		if e != nil {
			return counts, e
		}
		evraw, e := jsonValue(x.EventTypes)
		if e != nil {
			return counts, e
		}
		if err = insert("webhooks", "INSERT INTO webhooks(id,name,url,secret,enabled,workflow_ids,event_types,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)", x.ID, x.Name, x.URL, x.Secret, x.Enabled, wfraw, evraw, ms, ms); err != nil {
			return counts, err
		}
	}
	return counts, nil
}

func pf06InsertProxy(insert func(string, string, ...any) error, table string, x ProxyFixture, users, workflows map[string]string, ms int64) error {
	wf := make([]string, 0, len(x.WorkflowCodes))
	for _, c := range x.WorkflowCodes {
		wf = append(wf, workflows[c])
	}
	raw, err := json.Marshal(wf)
	if err != nil {
		return err
	}
	return insert(table, "INSERT INTO "+table+"(id,user_id,proxy_user_id,starts_on,ends_on,workflow_ids,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", x.ID, users[x.UserCode], users[x.ProxyUserCode], x.StartsOn, x.EndsOn, string(raw), ms, ms)
}

func pf06ClearFiles(root string) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
