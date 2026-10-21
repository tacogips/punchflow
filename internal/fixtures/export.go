package fixtures

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// Export reads fixture-owned rows and returns canonical JSON bytes by filename.
func Export(ctx context.Context, q store.Querier) (map[string][]byte, error) {
	s := &Set{}
	var t model.Tenant
	var allow, domains string
	var self bool
	var created, updated int64
	if err := q.QueryRowContext(ctx, "SELECT id,public_id,name,plan,mode,ip_allowlist,allowed_email_domains,self_service_proxy,created_at,updated_at FROM tenant").Scan(&t.ID, &t.PublicID, &t.Name, &t.Plan, &t.Mode, &allow, &domains, &self, &created, &updated); err != nil {
		return nil, fmt.Errorf("fixtures: export tenant: %w", err)
	}
	t.CreatedAt, t.UpdatedAt = store.Time(created).UTC(), store.Time(updated).UTC()
	var err error
	if err = json.Unmarshal([]byte(allow), &t.IPAllowlist); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(domains), &t.AllowedEmailDomains); err != nil {
		return nil, err
	}
	t.SelfServiceProxy = self
	s.Tenant = TenantFile{Tenant: t}
	if err = pf06Query(ctx, q, "SELECT id,code,name,level,is_default FROM grades ORDER BY code", func(r *sql.Rows) error {
		var x GradeFixture
		if e := r.Scan(&x.ID, &x.Code, &x.Name, &x.Level, &x.IsDefault); e != nil {
			return e
		}
		s.Grades = append(s.Grades, x)
		return nil
	}); err != nil {
		return nil, err
	}
	defs, err := store.UserCustomFieldList(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, d := range defs {
		s.UserCustomFields = append(s.UserCustomFields, UserCustomFieldFixture{ID: d.ID, Code: d.Code, Name: d.Name, FieldType: d.FieldType, Options: d.Options, VisibleToEveryone: d.VisibleToEveryone})
	}
	userByID := map[string]string{}
	if err = pf06Query(ctx, q, "SELECT id,code,email,first_name,last_name,employee_id,status,locale,user_type,custom_fields FROM users ORDER BY code", func(r *sql.Rows) error {
		var x UserFixture
		var employee sql.NullString
		var custom string
		if e := r.Scan(&x.ID, &x.Code, &x.Email, &x.FirstName, &x.LastName, &employee, &x.Status, &x.Locale, &x.UserType, &custom); e != nil {
			return e
		}
		if employee.Valid {
			x.EmployeeID = &employee.String
		}
		if e := json.Unmarshal([]byte(custom), &x.CustomFields); e != nil {
			return e
		}
		userByID[x.ID] = x.Code
		s.Users = append(s.Users, x)
		return nil
	}); err != nil {
		return nil, err
	}
	chartByID := map[string]int{}
	teamCodeByID := map[string]string{}
	gradeCodeByID := map[string]string{}
	for _, g := range s.Grades {
		gradeCodeByID[g.ID] = g.Code
	}
	if err = pf06Query(ctx, q, "SELECT id,name,current FROM organization_charts ORDER BY id", func(r *sql.Rows) error {
		var x OrgChartFixture
		if e := r.Scan(&x.ID, &x.Name, &x.Current); e != nil {
			return e
		}
		chartByID[x.ID] = len(s.OrgCharts)
		s.OrgCharts = append(s.OrgCharts, x)
		return nil
	}); err != nil {
		return nil, err
	}
	type teamRow struct {
		id, chart, code, name string
		notes, parent         sql.NullString
		approve               bool
	}
	teams := []teamRow{}
	if err = pf06Query(ctx, q, "SELECT id,organization_chart_id,code,name,notes,parent_id,approve_only FROM teams ORDER BY rowid", func(r *sql.Rows) error {
		var x teamRow
		if e := r.Scan(&x.id, &x.chart, &x.code, &x.name, &x.notes, &x.parent, &x.approve); e != nil {
			return e
		}
		teams = append(teams, x)
		teamCodeByID[x.id] = x.code
		return nil
	}); err != nil {
		return nil, err
	}
	for _, x := range teams {
		tf := TeamFixture{ID: x.id, Code: x.code, Name: x.name, ApproveOnly: x.approve}
		if x.notes.Valid {
			tf.Notes = &x.notes.String
		}
		if x.parent.Valid {
			p := teamCodeByID[x.parent.String]
			tf.ParentCode = &p
		}
		idx, ok := chartByID[x.chart]
		if !ok {
			return nil, fmt.Errorf("fixtures: team %s has unknown chart", x.code)
		}
		s.OrgCharts[idx].Teams = append(s.OrgCharts[idx].Teams, tf)
	}
	if err = pf06Query(ctx, q, "SELECT team_id,user_id,leader,grade_ids FROM team_memberships ORDER BY rowid", func(r *sql.Rows) error {
		var team, user string
		var leader bool
		var raw string
		if e := r.Scan(&team, &user, &leader, &raw); e != nil {
			return e
		}
		var gradeIDs []string
		if e := json.Unmarshal([]byte(raw), &gradeIDs); e != nil {
			return e
		}
		codes := make([]string, 0, len(gradeIDs))
		for _, id := range gradeIDs {
			codes = append(codes, gradeCodeByID[id])
		}
		for ci := range s.OrgCharts {
			for ti := range s.OrgCharts[ci].Teams {
				if s.OrgCharts[ci].Teams[ti].ID == team {
					s.OrgCharts[ci].Teams[ti].Memberships = append(s.OrgCharts[ci].Teams[ti].Memberships, MembershipFixture{UserCode: userByID[user], Leader: leader, GradeCodes: codes})
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	roleByID := map[string]int{}
	if err = pf06Query(ctx, q, "SELECT id,name,editable,permission_list FROM roles ORDER BY id", func(r *sql.Rows) error {
		var x RoleFixture
		var perms string
		if e := r.Scan(&x.ID, &x.Name, &x.Editable, &perms); e != nil {
			return e
		}
		if e := json.Unmarshal([]byte(perms), &x.PermissionList); e != nil {
			return e
		}
		roleByID[x.ID] = len(s.Roles)
		s.Roles = append(s.Roles, x)
		return nil
	}); err != nil {
		return nil, err
	}
	if err = pf06Query(ctx, q, "SELECT role_id,user_id FROM role_memberships ORDER BY rowid", func(r *sql.Rows) error {
		var role, user string
		if e := r.Scan(&role, &user); e != nil {
			return e
		}
		idx, ok := roleByID[role]
		if ok {
			s.Roles[idx].MemberCodes = append(s.Roles[idx].MemberCodes, userByID[user])
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err = pf06Query(ctx, q, "SELECT id,code,name FROM categories ORDER BY code", func(r *sql.Rows) error {
		var x CategoryFixture
		if e := r.Scan(&x.ID, &x.Code, &x.Name); e != nil {
			return e
		}
		s.Categories = append(s.Categories, x)
		return nil
	}); err != nil {
		return nil, err
	}
	folderByID := map[string]string{}
	type folderRow struct {
		x      FolderFixture
		parent sql.NullString
	}
	folderRows := []folderRow{}
	if err = pf06Query(ctx, q, "SELECT id,code,name,description,parent_id,editable FROM folders ORDER BY rowid", func(r *sql.Rows) error {
		var x folderRow
		var desc sql.NullString
		if e := r.Scan(&x.x.ID, &x.x.Code, &x.x.Name, &desc, &x.parent, &x.x.Editable); e != nil {
			return e
		}
		if desc.Valid {
			x.x.Description = &desc.String
		}
		folderByID[x.x.ID] = x.x.Code
		folderRows = append(folderRows, x)
		return nil
	}); err != nil {
		return nil, err
	}
	for _, x := range folderRows {
		if x.parent.Valid {
			p := folderByID[x.parent.String]
			x.x.ParentCode = &p
		}
		s.Folders = append(s.Folders, x.x)
	}
	if err = pf06Query(ctx, q, "SELECT id,code,name,description,default_sort_by,initial_display_code,initial_display_created_at,initial_display_description FROM general_masters ORDER BY code", func(r *sql.Rows) error {
		var x GeneralMasterFixture
		var desc sql.NullString
		if e := r.Scan(&x.ID, &x.Code, &x.Name, &desc, &x.DefaultSortBy, &x.InitialDisplayCode, &x.InitialDisplayCreatedAt, &x.InitialDisplayDescription); e != nil {
			return e
		}
		if desc.Valid {
			x.Description = &desc.String
		}
		s.GeneralMasters = append(s.GeneralMasters, x)
		return nil
	}); err != nil {
		return nil, err
	}
	masterIdx := map[string]int{}
	for i, x := range s.GeneralMasters {
		masterIdx[x.ID] = i
	}
	fieldCode := map[string]map[string]string{}
	if err = pf06Query(ctx, q, "SELECT id,general_master_id,code,title,description,field_type,required,field_order,visible,initial_display,options FROM general_master_fields ORDER BY rowid", func(r *sql.Rows) error {
		var x MasterFieldFixture
		var mid string
		var desc, opts sql.NullString
		if e := r.Scan(&x.ID, &mid, &x.Code, &x.Title, &desc, &x.FieldType, &x.Required, &x.FieldOrder, &x.Visible, &x.InitialDisplay, &opts); e != nil {
			return e
		}
		if desc.Valid {
			x.Description = &desc.String
		}
		if opts.Valid {
			if e := json.Unmarshal([]byte(opts.String), &x.Options); e != nil {
				return e
			}
		}
		idx := masterIdx[mid]
		s.GeneralMasters[idx].Fields = append(s.GeneralMasters[idx].Fields, x)
		if fieldCode[mid] == nil {
			fieldCode[mid] = map[string]string{}
		}
		fieldCode[mid][x.ID] = x.Code
		return nil
	}); err != nil {
		return nil, err
	}
	if err = pf06Query(ctx, q, "SELECT id,general_master_id,code,name,description,starts_on,ends_on,inputs,input_ids FROM general_master_items ORDER BY rowid", func(r *sql.Rows) error {
		var x MasterItemFixture
		var mid, inputs, inputIDsRaw string
		var desc, starts, ends sql.NullString
		if e := r.Scan(&x.ID, &mid, &x.Code, &x.Name, &desc, &starts, &ends, &inputs, &inputIDsRaw); e != nil {
			return e
		}
		if desc.Valid {
			x.Description = &desc.String
		}
		if starts.Valid {
			x.StartsOn = &starts.String
		}
		if ends.Valid {
			x.EndsOn = &ends.String
		}
		var values map[string]any
		if e := json.Unmarshal([]byte(inputs), &values); e != nil {
			return e
		}
		var inputIDs map[string]string
		if e := json.Unmarshal([]byte(inputIDsRaw), &inputIDs); e != nil {
			return fmt.Errorf("fixtures: export master item %s input IDs: %w", x.Code, e)
		}
		x.Inputs = make([]MasterInputFixture, 0, len(values))
		for id, v := range values {
			code, ok := fieldCode[mid][id]
			if !ok {
				return fmt.Errorf("fixtures: export master item %s: unknown field ID %s", x.Code, id)
			}
			inputID, ok := inputIDs[id]
			if !ok || inputID == "" {
				return fmt.Errorf("fixtures: export master item %s: missing input ID for field %s", x.Code, code)
			}
			x.Inputs = append(x.Inputs, MasterInputFixture{ID: inputID, FieldCode: code, Value: v})
		}
		sort.SliceStable(x.Inputs, func(i, j int) bool { return x.Inputs[i].FieldCode < x.Inputs[j].FieldCode })
		s.GeneralMasters[masterIdx[mid]].Items = append(s.GeneralMasters[masterIdx[mid]].Items, x)
		return nil
	}); err != nil {
		return nil, err
	}
	if err = pf06Query(ctx, q, "SELECT definition_json FROM routes ORDER BY code", func(r *sql.Rows) error {
		var raw string
		if e := r.Scan(&raw); e != nil {
			return e
		}
		var x model.RouteDef
		if e := json.Unmarshal([]byte(raw), &x); e != nil {
			return e
		}
		s.Routes = append(s.Routes, x)
		return nil
	}); err != nil {
		return nil, err
	}
	if err = pf06Query(ctx, q, "SELECT definition_json FROM workflows ORDER BY code", func(r *sql.Rows) error {
		var raw string
		if e := r.Scan(&raw); e != nil {
			return e
		}
		var x model.WorkflowDef
		if e := json.Unmarshal([]byte(raw), &x); e != nil {
			return e
		}
		s.Workflows = append(s.Workflows, x)
		return nil
	}); err != nil {
		return nil, err
	}
	workflowCode := map[string]string{}
	if err = pf06Query(ctx, q, "SELECT id,code FROM workflows", func(r *sql.Rows) error {
		var id, code string
		if e := r.Scan(&id, &code); e != nil {
			return e
		}
		workflowCode[id] = code
		return nil
	}); err != nil {
		return nil, err
	}
	for _, table := range []struct {
		name string
		dst  *[]ProxyFixture
	}{{"proxy_applicants", &s.ProxyApplicants}, {"proxy_approvers", &s.ProxyApprovers}} {
		query := "SELECT id,user_id,proxy_user_id,starts_on,ends_on,workflow_ids FROM " + table.name + " ORDER BY id"
		if err = pf06Query(ctx, q, query, func(r *sql.Rows) error {
			var x ProxyFixture
			var user, proxy, raw string
			var start, end sql.NullString
			if e := r.Scan(&x.ID, &user, &proxy, &start, &end, &raw); e != nil {
				return e
			}
			x.UserCode = userByID[user]
			x.ProxyUserCode = userByID[proxy]
			if start.Valid {
				x.StartsOn = &start.String
			}
			if end.Valid {
				x.EndsOn = &end.String
			}
			var ids []string
			if e := json.Unmarshal([]byte(raw), &ids); e != nil {
				return e
			}
			for _, id := range ids {
				x.WorkflowCodes = append(x.WorkflowCodes, workflowCode[id])
			}
			*table.dst = append(*table.dst, x)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	if err = pf06Query(ctx, q, "SELECT id,kind,name,description,user_id,fixture_plaintext FROM api_tokens WHERE fixture_plaintext IS NOT NULL ORDER BY id", func(r *sql.Rows) error {
		var x TokenFixture
		var desc, user sql.NullString
		if e := r.Scan(&x.ID, &x.Kind, &x.Name, &desc, &user, &x.Token); e != nil {
			return e
		}
		if desc.Valid {
			x.Description = &desc.String
		}
		if user.Valid {
			x.UserCode = userByID[user.String]
		}
		s.Tokens = append(s.Tokens, x)
		return nil
	}); err != nil {
		return nil, err
	}
	if err = pf06Query(ctx, q, "SELECT id,name,url,secret,enabled,workflow_ids,event_types FROM webhooks ORDER BY id", func(r *sql.Rows) error {
		var x WebhookFixture
		var secret, workflows, events string
		if e := r.Scan(&x.ID, &x.Name, &x.URL, &secret, &x.Enabled, &workflows, &events); e != nil {
			return e
		}
		if e := json.Unmarshal([]byte(events), &x.EventTypes); e != nil {
			return e
		}
		var ids []string
		if e := json.Unmarshal([]byte(workflows), &ids); e != nil {
			return e
		}
		for _, id := range ids {
			x.WorkflowCodes = append(x.WorkflowCodes, workflowCode[id])
		}
		x.Secret = secret
		s.Webhooks = append(s.Webhooks, x)
		return nil
	}); err != nil {
		return nil, err
	}
	return pf06EncodeSet(s)
}

func pf06Query(ctx context.Context, q store.Querier, query string, fn func(*sql.Rows) error) error {
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("fixtures: export query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return fmt.Errorf("fixtures: export row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("fixtures: export rows: %w", err)
	}
	return nil
}

func pf06EncodeSet(s *Set) (map[string][]byte, error) {
	if s.Grades == nil {
		s.Grades = []GradeFixture{}
	}
	if s.Users == nil {
		s.Users = []UserFixture{}
	}
	if s.OrgCharts == nil {
		s.OrgCharts = []OrgChartFixture{}
	}
	if s.Roles == nil {
		s.Roles = []RoleFixture{}
	}
	if s.Categories == nil {
		s.Categories = []CategoryFixture{}
	}
	if s.UserCustomFields == nil {
		s.UserCustomFields = []UserCustomFieldFixture{}
	}
	if s.Folders == nil {
		s.Folders = []FolderFixture{}
	}
	if s.GeneralMasters == nil {
		s.GeneralMasters = []GeneralMasterFixture{}
	}
	if s.Routes == nil {
		s.Routes = []model.RouteDef{}
	}
	if s.Workflows == nil {
		s.Workflows = []model.WorkflowDef{}
	}
	if s.ProxyApplicants == nil {
		s.ProxyApplicants = []ProxyFixture{}
	}
	if s.ProxyApprovers == nil {
		s.ProxyApprovers = []ProxyFixture{}
	}
	if s.Tokens == nil {
		s.Tokens = []TokenFixture{}
	}
	if s.Webhooks == nil {
		s.Webhooks = []WebhookFixture{}
	}
	values := []struct {
		name string
		v    any
	}{{FileTenant, s.Tenant}, {FileGrades, s.Grades}, {FileUsers, s.Users}, {FileUserCustomFields, s.UserCustomFields}, {FileOrganizationCharts, s.OrgCharts}, {FileRoles, s.Roles}, {FileCategories, s.Categories}, {FileFolders, s.Folders}, {FileGeneralMasters, s.GeneralMasters}, {FileRoutes, s.Routes}, {FileWorkflows, s.Workflows}, {FileProxyApplicants, s.ProxyApplicants}, {FileProxyApprovers, s.ProxyApprovers}, {FileTokens, s.Tokens}, {FileWebhooks, s.Webhooks}}
	for i := range values {
		if values[i].v == nil {
			values[i].v = []any{}
		}
	}
	out := map[string][]byte{}
	for _, x := range values {
		b, err := json.MarshalIndent(x.v, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("fixtures: encode %s: %w", x.name, err)
		}
		out[x.name] = append(b, '\n')
	}
	return out, nil
}

// WriteDir writes canonical fixture files into dir, replacing each named file.
func WriteDir(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("fixtures: create directory: %w", err)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		if filepath.Base(name) != name {
			return fmt.Errorf("fixtures: invalid output filename %q", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		tmp, err := os.CreateTemp(dir, ".fixture-*")
		if err != nil {
			return err
		}
		temp := tmp.Name()
		if _, err = tmp.Write(files[name]); err != nil {
			_ = tmp.Close()
			_ = os.Remove(temp)
			return err
		}
		if err = tmp.Sync(); err != nil {
			_ = tmp.Close()
			_ = os.Remove(temp)
			return err
		}
		if err = tmp.Close(); err != nil {
			_ = os.Remove(temp)
			return err
		}
		if err = os.Rename(temp, filepath.Join(dir, name)); err != nil {
			_ = os.Remove(temp)
			return err
		}
	}
	return nil
}

// DefaultFS returns the embedded default fixture sub-filesystem.
func DefaultFS(root fs.FS) (fs.FS, error) {
	sub, err := fs.Sub(root, "fixtures/default")
	if err != nil {
		return nil, fmt.Errorf("fixtures: open default fixtures: %w", err)
	}
	return sub, nil
}
