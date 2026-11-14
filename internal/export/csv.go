// Package export writes punchflow's CSV downloads (PF-21): the ticket and
// detail-row layouts of a ticket search result, and audit logs.
//
// Files are UTF-8 with a byte order mark and CRLF line endings so that Excel
// opens Japanese text correctly (an inference: kickflow's manual does not state
// the encoding). Cells that spreadsheet applications would evaluate as formulas
// are neutralised with a leading apostrophe.
package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ContentType is the media type of every export.
const ContentType = "text/csv; charset=utf-8"

const byteOrderMark = "\xEF\xBB\xBF"

// Writer writes spreadsheet-safe CSV rows.
type Writer struct {
	csv *csv.Writer
}

// NewWriter writes the byte order mark to w and returns a Writer.
func NewWriter(w io.Writer) (*Writer, error) {
	if _, err := io.WriteString(w, byteOrderMark); err != nil {
		return nil, fmt.Errorf("export: write byte order mark: %w", err)
	}
	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	return &Writer{csv: cw}, nil
}

// Write sanitises and writes one row.
func (w *Writer) Write(row []string) error {
	out := make([]string, len(row))
	for i, cell := range row {
		out[i] = SanitizeCell(cell)
	}
	if err := w.csv.Write(out); err != nil {
		return fmt.Errorf("export: write csv row: %w", err)
	}
	return nil
}

// Flush flushes buffered rows and reports any write error.
func (w *Writer) Flush() error {
	w.csv.Flush()
	if err := w.csv.Error(); err != nil {
		return fmt.Errorf("export: flush csv: %w", err)
	}
	return nil
}

// SanitizeCell neutralises spreadsheet formula injection. A cell starting with
// =, +, @, tab or carriage return, or with - unless it is a plain number, gets a
// leading apostrophe.
func SanitizeCell(cell string) string {
	if cell == "" {
		return cell
	}
	switch cell[0] {
	case '=', '+', '@', '\t', '\r':
		return "'" + cell
	case '-':
		if _, err := strconv.ParseFloat(strings.ReplaceAll(cell, ",", ""), 64); err == nil {
			return cell
		}
		return "'" + cell
	}
	return cell
}
