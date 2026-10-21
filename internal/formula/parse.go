// Package formula parses and evaluates the calculation-field expressions of
// punchflow workflows.
//
// kickflow does not publish its formula grammar, so this package implements a
// documented, conservative subset (see Parse). Expressions outside the subset
// fail at Parse time with an *UnsupportedError so callers can report them
// instead of silently storing null.
package formula

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// UnsupportedError reports an expression outside the supported subset.
type UnsupportedError struct{ Reason string }

func (e *UnsupportedError) Error() string { return "unsupported formula: " + e.Reason }

func unsupported(format string, args ...any) error {
	return &UnsupportedError{Reason: fmt.Sprintf(format, args...)}
}

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokNumber
	tokString
	tokRef
	tokIdent
	tokOp
)

type token struct {
	kind tokenKind
	text string
	num  float64
}

// Expr is a parsed formula.
type Expr struct{ root node }

// Refs returns the distinct field references used by the expression in first
// use order. Function names are not references.
func (e *Expr) Refs() []string {
	seen := map[string]bool{}
	out := []string{}
	var walk func(n node)
	walk = func(n node) {
		switch v := n.(type) {
		case refNode:
			if !seen[v.name] {
				seen[v.name] = true
				out = append(out, v.name)
			}
		case unaryNode:
			walk(v.operand)
		case binaryNode:
			walk(v.left)
			walk(v.right)
		case callNode:
			for _, a := range v.args {
				walk(a)
			}
		}
	}
	walk(e.root)
	return out
}

// Parse compiles a formula.
//
// Supported syntax (inferred, see package docs):
//   - numbers (12, 3.5), strings ("text" or 'text'), TRUE and FALSE
//   - field references: bare identifiers (price, lines.qty) or {any-code} /
//     [any-code] for codes containing characters such as '-'
//   - arithmetic + - * / % with parentheses and unary minus
//   - string concatenation with & and CONCAT(...)
//   - comparisons < <= > >= = == != <> and logic && || ! (also AND, OR, NOT)
//   - SUM AVG AVERAGE MIN MAX COUNT ROUND ROUNDUP ROUNDDOWN FLOOR CEIL
//     CEILING ABS IF AND OR NOT CONCAT
//
// A reference named "section.field" (or a bare detail field code) is a detail
// column and evaluates to the list of that field's values across rows.
func Parse(src string) (*Expr, error) {
	if strings.TrimSpace(src) == "" {
		return nil, unsupported("empty expression")
	}
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	n, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokEOF {
		return nil, unsupported("unexpected %q", p.peek().text)
	}
	return &Expr{root: n}, nil
}

func tokenize(src string) ([]token, error) {
	rs := []rune(src)
	var toks []token
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case unicode.IsDigit(c) || c == '.' && i+1 < len(rs) && unicode.IsDigit(rs[i+1]):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.') {
				j++
			}
			n, err := strconv.ParseFloat(string(rs[i:j]), 64)
			if err != nil {
				return nil, unsupported("invalid number %q", string(rs[i:j]))
			}
			toks = append(toks, token{kind: tokNumber, text: string(rs[i:j]), num: n})
			i = j
		case c == '"' || c == '\'':
			j := i + 1
			var sb strings.Builder
			for ; j < len(rs) && rs[j] != c; j++ {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
				}
				sb.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, unsupported("unterminated string")
			}
			toks = append(toks, token{kind: tokString, text: sb.String()})
			i = j + 1
		case c == '{' || c == '[':
			closer := '}'
			if c == '[' {
				closer = ']'
			}
			j := i + 1
			for j < len(rs) && rs[j] != closer {
				j++
			}
			if j >= len(rs) || j == i+1 {
				return nil, unsupported("malformed reference")
			}
			toks = append(toks, token{kind: tokRef, text: strings.TrimSpace(string(rs[i+1 : j]))})
			i = j + 1
		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_' || rs[j] == '.') {
				j++
			}
			toks = append(toks, token{kind: tokIdent, text: string(rs[i:j])})
			i = j
		default:
			op := string(c)
			if i+1 < len(rs) {
				switch two := string(rs[i : i+2]); two {
				case "<=", ">=", "==", "!=", "<>", "&&", "||":
					op = two
				}
			}
			if len(op) == 1 && !strings.Contains("+-*/%(),<>=!&", op) {
				return nil, unsupported("unexpected character %q", op)
			}
			toks = append(toks, token{kind: tokOp, text: op})
			i += len([]rune(op))
		}
	}
	return append(toks, token{kind: tokEOF}), nil
}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token {
	t := p.toks[p.pos]
	if t.kind != tokEOF {
		p.pos++
	}
	return t
}
func (p *parser) isOp(texts ...string) (string, bool) {
	t := p.peek()
	if t.kind != tokOp {
		return "", false
	}
	for _, s := range texts {
		if t.text == s {
			return s, true
		}
	}
	return "", false
}

func (p *parser) binary(sub func() (node, error), ops ...string) (node, error) {
	left, err := sub()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.isOp(ops...)
		if !ok {
			return left, nil
		}
		p.next()
		right, err := sub()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: op, left: left, right: right}
	}
}

func (p *parser) parseOr() (node, error)  { return p.binary(p.parseAnd, "||") }
func (p *parser) parseAnd() (node, error) { return p.binary(p.parseCmp, "&&") }
func (p *parser) parseCmp() (node, error) {
	return p.binary(p.parseConcat, "<", "<=", ">", ">=", "=", "==", "!=", "<>")
}
func (p *parser) parseConcat() (node, error) { return p.binary(p.parseAdd, "&") }
func (p *parser) parseAdd() (node, error)    { return p.binary(p.parseMul, "+", "-") }
func (p *parser) parseMul() (node, error)    { return p.binary(p.parseUnary, "*", "/", "%") }

func (p *parser) parseUnary() (node, error) {
	if op, ok := p.isOp("-", "!", "+"); ok {
		p.next()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return unaryNode{op: op, operand: operand}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parseCall(name string) (node, error) {
	fn, known := functions[strings.ToUpper(name)]
	if !known {
		return nil, unsupported("function %s", name)
	}
	p.next() // (
	args := []node{}
	if _, ok := p.isOp(")"); !ok {
		for {
			a, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			args = append(args, a)
			if _, ok := p.isOp(","); !ok {
				break
			}
			p.next()
		}
	}
	if _, ok := p.isOp(")"); !ok {
		return nil, unsupported("missing ) after %s arguments", name)
	}
	p.next()
	if len(args) < fn.minArgs || fn.maxArgs >= 0 && len(args) > fn.maxArgs {
		return nil, unsupported("wrong argument count for %s", name)
	}
	return callNode{name: strings.ToUpper(name), args: args}, nil
}

func (p *parser) parsePrimary() (node, error) {
	t := p.next()
	switch t.kind {
	case tokNumber:
		return literalNode{value: t.num}, nil
	case tokString:
		return literalNode{value: t.text}, nil
	case tokRef:
		return refNode{name: t.text}, nil
	case tokIdent:
		if _, ok := p.isOp("("); ok {
			return p.parseCall(t.text)
		}
		switch strings.ToUpper(t.text) {
		case "TRUE":
			return literalNode{value: true}, nil
		case "FALSE":
			return literalNode{value: false}, nil
		}
		return refNode{name: t.text}, nil
	case tokOp:
		if t.text == "(" {
			n, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			if _, ok := p.isOp(")"); !ok {
				return nil, unsupported("missing )")
			}
			p.next()
			return n, nil
		}
	}
	if t.kind == tokEOF {
		return nil, unsupported("unexpected end of expression")
	}
	return nil, unsupported("unexpected %q", t.text)
}
