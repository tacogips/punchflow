// Package ticketsearch implements punchflow's free-text ticket keyword search
// (PF-22). It matches ticket numbers, issued custom numbers, titles, form input
// values, detail row cells and comments, and never matches text of any
// restricted form section.
// Ticket visibility itself is applied by the caller.
package ticketsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"punchflow/internal/access"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

const (
	// MaxTerms bounds the number of whitespace-separated terms honoured.
	MaxTerms = 8
	// MaxTermRunes bounds the length of a single term.
	MaxTermRunes = 100
)

// Terms splits a keyword into whitespace-separated terms (full-width spaces
// included). A ticket must contain every term (AND semantics).
func Terms(keyword string) []string {
	fields := strings.Fields(strings.ReplaceAll(keyword, "　", " "))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if utf8.RuneCountInString(field) > MaxTermRunes {
			field = string([]rune(field)[:MaxTermRunes])
		}
		out = append(out, field)
		if len(out) == MaxTerms {
			break
		}
	}
	return out
}

// Match returns the set of ticket IDs that match the keyword for the caller.
// An empty keyword matches nothing; callers decide how to treat it. The result
// is not narrowed by ticket visibility.
func Match(ctx context.Context, q store.Querier, caller access.Caller, keyword string) (map[string]bool, error) {
	terms := Terms(keyword)
	matched := map[string]bool{}
	if len(terms) == 0 {
		return matched, nil
	}
	candidates, err := store.TicketKeywordCandidates(ctx, q, terms)
	if err != nil {
		return nil, err
	}
	defs := map[string]model.WorkflowDef{}
	for _, id := range candidates {
		ticket, err := store.EngLoadTicket(ctx, q, id)
		if err != nil {
			return nil, fmt.Errorf("ticketsearch: load ticket: %w", err)
		}
		def, ok := defs[ticket.WorkflowID]
		if !ok {
			if def, err = store.WorkflowDefByID(ctx, q, ticket.WorkflowID); err != nil {
				return nil, fmt.Errorf("ticketsearch: load workflow: %w", err)
			}
			defs[ticket.WorkflowID] = def
		}
		hidden := restrictedFieldIDs(def)
		if len(hidden) == 0 {
			matched[id] = true
			continue
		}
		ok, err = matchesVisible(ctx, q, ticket, hidden, terms)
		if err != nil {
			return nil, err
		}
		if ok {
			matched[id] = true
		}
	}
	return matched, nil
}

// restrictedFieldIDs returns every form field ID belonging to a restricted
// section. INF-SEC-01: keyword search excludes these fields for every caller,
// including section viewers and ticket_read administrators.
func restrictedFieldIDs(def model.WorkflowDef) map[string]bool {
	hidden := map[string]bool{}
	for _, section := range def.SectionList {
		if section.SectionType != model.SectionTypeForm || !section.ViewRestriction {
			continue
		}
		for _, field := range section.FormFields {
			hidden[field.ID] = true
		}
	}
	if len(hidden) == 0 {
		return nil
	}
	return hidden
}

// matchesVisible re-evaluates the keyword without restricted form fields.
func matchesVisible(ctx context.Context, q store.Querier, ticket model.Ticket, hidden map[string]bool, terms []string) (bool, error) {
	haystacks := []string{}
	if ticket.TicketNumber != nil {
		haystacks = append(haystacks, *ticket.TicketNumber)
	}
	customNumbers, err := store.TicketCustomNumberingValues(ctx, q, ticket.ID)
	if err != nil {
		return false, err
	}
	haystacks = append(haystacks, customNumbers...)
	if ticket.Title != nil {
		haystacks = append(haystacks, *ticket.Title)
	}
	inputs, err := store.TicketInputs(ctx, q, ticket.ID)
	if err != nil {
		return false, fmt.Errorf("ticketsearch: load inputs: %w", err)
	}
	for _, input := range inputs {
		if !hidden[input.FormFieldID] {
			haystacks = append(haystacks, valueText(input.Value))
		}
	}
	slips, err := store.TicketSlipItems(ctx, q, ticket.ID)
	if err != nil {
		return false, fmt.Errorf("ticketsearch: load detail rows: %w", err)
	}
	for _, slip := range slips {
		for _, cell := range slip.Inputs {
			haystacks = append(haystacks, valueText(cell))
		}
	}
	comments, err := store.TicketLiveCommentBodies(ctx, q, ticket.ID)
	if err != nil {
		return false, err
	}
	haystacks = append(haystacks, comments...)
	for i := range haystacks {
		haystacks[i] = strings.ToLower(haystacks[i])
	}
	for _, term := range terms {
		term = strings.ToLower(term)
		found := false
		for _, haystack := range haystacks {
			if strings.Contains(haystack, term) {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

func valueText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	}
}
