package fixtures

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"

	"punchflow/internal/model"
)

// Set is one complete input collection of fixture files.
type Set struct {
	Tenant           TenantFile
	Grades           []GradeFixture
	Users            []UserFixture
	UserCustomFields []UserCustomFieldFixture
	OrgCharts        []OrgChartFixture
	Roles            []RoleFixture
	Categories       []CategoryFixture
	Folders          []FolderFixture
	GeneralMasters   []GeneralMasterFixture
	Routes           []model.RouteDef
	Workflows        []model.WorkflowDef
	ProxyApplicants  []ProxyFixture
	ProxyApprovers   []ProxyFixture
	Tokens           []TokenFixture
	Webhooks         []WebhookFixture
	Labels           []LabelFixture
	SubStatuses      []SubStatusFixture
}

// Problem describes one invalid value in a fixture file.
type Problem struct{ File, Path, Message string }

// ValidationError contains all fixture validation problems in stable order.
type ValidationError struct{ Problems []Problem }

func (e *ValidationError) Error() string {
	lines := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		lines = append(lines, fmt.Sprintf("%s: %s: %s", p.File, p.Path, p.Message))
	}
	return strings.Join(lines, "\n")
}

// Parse reads the known fixture files at fsys' root. Required files must exist;
// optional files default to empty collections, and unrelated files are ignored.
func Parse(fsys fs.FS) (*Set, error) {
	out := &Set{}
	files := []struct {
		name string
		dst  any
		req  bool
	}{
		{FileTenant, &out.Tenant, true}, {FileGrades, &out.Grades, true},
		{FileUsers, &out.Users, true}, {FileUserCustomFields, &out.UserCustomFields, false},
		{FileOrganizationCharts, &out.OrgCharts, true},
		{FileRoles, &out.Roles, true}, {FileCategories, &out.Categories, false},
		{FileFolders, &out.Folders, true}, {FileGeneralMasters, &out.GeneralMasters, false},
		{FileRoutes, &out.Routes, false}, {FileWorkflows, &out.Workflows, false},
		{FileProxyApplicants, &out.ProxyApplicants, false}, {FileProxyApprovers, &out.ProxyApprovers, false},
		{FileTokens, &out.Tokens, false}, {FileWebhooks, &out.Webhooks, false},
		{FileLabels, &out.Labels, false}, {FileSubStatuses, &out.SubStatuses, false},
	}
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f.name)
		if errors.Is(err, fs.ErrNotExist) {
			if f.req {
				return nil, fmt.Errorf("fixtures: required file %s is missing", f.name)
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("fixtures: read %s: %w", f.name, err)
		}
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(f.dst); err != nil {
			if strings.Contains(err.Error(), "unknown field") {
				name := strings.Trim(strings.TrimPrefix(strings.SplitN(err.Error(), "unknown field ", 2)[1], "\""), "\"")
				return nil, &ValidationError{Problems: []Problem{{File: f.name, Path: name, Message: "unknown field"}}}
			}
			return nil, fmt.Errorf("fixtures: decode %s: %w", f.name, err)
		}
		var extra any
		if err := dec.Decode(&extra); err == nil {
			return nil, fmt.Errorf("fixtures: decode %s: trailing JSON value", f.name)
		} else if !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("fixtures: decode %s: %w", f.name, err)
		}
	}
	return out, nil
}

// sortProblems sorts validation errors by file and path for stable output.
func sortProblems(problems []Problem) {
	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].File != problems[j].File {
			return problems[i].File < problems[j].File
		}
		if problems[i].Path != problems[j].Path {
			return problems[i].Path < problems[j].Path
		}
		return problems[i].Message < problems[j].Message
	})
}
