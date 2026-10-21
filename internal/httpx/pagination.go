package httpx

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"punchflow/internal/kferr"
)

// Page represents a one-based page request.
type Page struct{ Page, PerPage int }

// ParsePage parses page and perPage, applying kickflow defaults and bounds.
func ParsePage(r *http.Request) (Page, error) {
	p := Page{Page: 1, PerPage: 25}
	for key, target := range map[string]*int{"page": &p.Page, "perPage": &p.PerPage} {
		values, ok := r.URL.Query()[key]
		if !ok {
			continue
		}
		if len(values) != 1 {
			return Page{}, kferr.InvalidParameter("Invalid " + key)
		}
		n, err := strconv.Atoi(values[0])
		if err != nil {
			return Page{}, kferr.InvalidParameter("Invalid " + key)
		}
		*target = n
	}
	if p.Page < 1 {
		return Page{}, kferr.InvalidParameter("Invalid page")
	}
	if p.PerPage < 1 || p.PerPage > 100 {
		return Page{}, kferr.InvalidParameter("Invalid perPage")
	}
	if p.Page-1 > math.MaxInt/p.PerPage {
		return Page{}, kferr.InvalidParameter("Invalid page")
	}
	return p, nil
}

// Offset returns the zero-based row offset.
func (p Page) Offset() int { return (p.Page - 1) * p.PerPage }

// SetPagination writes pagination metadata and applicable absolute Link relations.
func SetPagination(w http.ResponseWriter, r *http.Request, baseURL string, p Page, total int) {
	last := int(math.Ceil(float64(total) / float64(p.PerPage)))
	if last < 1 {
		last = 1
	}
	rels := make([]string, 0, 4)
	for _, rel := range []struct {
		name       string
		page       int
		applicable bool
	}{
		{"first", 1, p.Page > 1}, {"prev", p.Page - 1, p.Page > 1}, {"next", p.Page + 1, p.Page < last}, {"last", last, p.Page < last},
	} {
		if !rel.applicable {
			continue
		}
		q := r.URL.Query()
		q.Set("page", strconv.Itoa(rel.page))
		q.Set("perPage", strconv.Itoa(p.PerPage))
		u := strings.TrimRight(baseURL, "/") + r.URL.Path
		if encoded := q.Encode(); encoded != "" {
			u += "?" + encoded
		}
		rels = append(rels, fmt.Sprintf("<%s>; rel=\"%s\"", u, rel.name))
	}
	if len(rels) > 0 {
		w.Header().Set("Link", strings.Join(rels, ", "))
	}
	w.Header().Set("Page", strconv.Itoa(p.Page))
	w.Header().Set("Per-Page", strconv.Itoa(p.PerPage))
	w.Header().Set("Total", strconv.Itoa(total))
}

// Sort is a validated sortBy request.
type Sort struct {
	Field string
	Desc  bool
	Set   bool
}

// ParseSort parses sortBy and ignores unsupported fields or suffixes.
func ParseSort(r *http.Request, allowed ...string) Sort {
	value := r.URL.Query().Get("sortBy")
	if value == "" {
		return Sort{}
	}
	desc := false
	field := value
	if i := strings.LastIndexByte(value, '-'); i >= 0 {
		field = value[:i]
		switch value[i+1:] {
		case "asc":
		case "desc":
			desc = true
		default:
			return Sort{}
		}
	}
	for _, candidate := range allowed {
		if field == candidate {
			return Sort{Field: field, Desc: desc, Set: true}
		}
	}
	return Sort{}
}
