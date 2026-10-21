package fixtures

import (
	"fmt"
	"regexp"
	"strings"

	"punchflow/internal/model"
)

var labelColorPattern = regexp.MustCompile(`^[a-f0-9]{6}$`)

// validateLabelsAndSubStatuses checks the label and sub-status files and every
// reference to a sub-status code from routes and workflows.
func validateLabelsAndSubStatuses(s *Set, add func(file, path, msg string)) {
	labelNames := map[string]int{}
	for i, l := range s.Labels {
		p := fmt.Sprintf("[%d]", i)
		if strings.TrimSpace(l.Name) == "" {
			add(FileLabels, p+".name", "name is required")
		} else if prior, dup := labelNames[l.Name]; dup {
			add(FileLabels, p+".name", fmt.Sprintf("duplicate name (also used at [%d].name)", prior))
		} else {
			labelNames[l.Name] = i
		}
		if !labelColorPattern.MatchString(l.Color) {
			add(FileLabels, p+".color", "color must be a six digit lowercase hex code without #")
		}
	}
	codes := map[string]int{}
	for i, x := range s.SubStatuses {
		p := fmt.Sprintf("[%d]", i)
		if strings.TrimSpace(x.Name) == "" {
			add(FileSubStatuses, p+".name", "name is required")
		}
		if strings.TrimSpace(x.Code) == "" {
			add(FileSubStatuses, p+".code", "code is required")
		} else if prior, dup := codes[x.Code]; dup {
			add(FileSubStatuses, p+".code", fmt.Sprintf("duplicate code (also used at [%d].code)", prior))
		} else {
			codes[x.Code] = i
		}
	}
	known := func(code string) bool { _, ok := codes[code]; return ok }
	for i, r := range s.Routes {
		for j, step := range r.Steps {
			if step.SubStatus != nil && !known(*step.SubStatus) {
				add(FileRoutes, fmt.Sprintf("[%d].steps[%d].subStatus", i, j), "unknown sub-status code")
			}
		}
	}
	for i, w := range s.Workflows {
		p := fmt.Sprintf("[%d].ticketNumberingSubStatus", i)
		switch {
		case w.TicketNumberingSubStatus != nil && w.TicketNumberingTiming != model.TicketNumberingTimingSubStatusAttached:
			add(FileWorkflows, p, "only allowed when ticketNumberingTiming is sub_status_attached")
		case w.TicketNumberingSubStatus != nil && !known(*w.TicketNumberingSubStatus):
			add(FileWorkflows, p, "unknown sub-status code")
		}
	}
}
