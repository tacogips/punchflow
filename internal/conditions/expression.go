package conditions

import (
	"fmt"
	"punchflow/internal/model"
	"unicode"
)

// MatchExpression evaluates condition fields using Kickflow's A-J expression
// syntax. An empty combination uses the existing all/any behavior.
func MatchExpression(fields []model.ConditionField, combination, expression string, values map[string]any) bool {
	if combination != "custom" {
		return Match(fields, combination, values)
	}
	matched := make([]bool, len(fields))
	for i, field := range fields {
		key := field.Variable
		if key == "" {
			key = field.FieldKey
		}
		actual, ok := Value(values, key)
		matched[i] = ok && Compare(actual, field.Symbol, field.Value)
	}
	result, _, err := parseExpression(expression, len(fields), func(index int) bool { return matched[index] })
	return err == nil && result
}

// ValidateExpression checks an A-J condition expression and its references.
func ValidateExpression(expression string, conditionCount int) error {
	if conditionCount > 10 {
		return fmt.Errorf("custom expressions support at most ten condition fields")
	}
	_, _, err := parseExpression(expression, conditionCount, func(int) bool { return false })
	return err
}

type expressionParser struct {
	runes []rune
	pos   int
	count int
	value func(int) bool
	refs  map[int]bool
}

func parseExpression(expression string, count int, value func(int) bool) (bool, map[int]bool, error) {
	p := expressionParser{runes: []rune(expression), count: count, value: value, refs: map[int]bool{}}
	result, err := p.parseOr()
	if err != nil {
		return false, p.refs, err
	}
	p.space()
	if p.pos != len(p.runes) {
		return false, p.refs, fmt.Errorf("unexpected token at position %d", p.pos)
	}
	return result, p.refs, nil
}

func (p *expressionParser) parseOr() (bool, error) {
	value, err := p.parseAnd()
	if err != nil {
		return false, err
	}
	for {
		p.space()
		if !p.take("||") {
			return value, nil
		}
		right, err := p.parseAnd()
		if err != nil {
			return false, err
		}
		value = value || right
	}
}

func (p *expressionParser) parseAnd() (bool, error) {
	value, err := p.parseAtom()
	if err != nil {
		return false, err
	}
	for {
		p.space()
		if !p.take("&&") {
			return value, nil
		}
		right, err := p.parseAtom()
		if err != nil {
			return false, err
		}
		value = value && right
	}
}

func (p *expressionParser) parseAtom() (bool, error) {
	p.space()
	if p.take("(") {
		value, err := p.parseOr()
		if err != nil {
			return false, err
		}
		p.space()
		if !p.take(")") {
			return false, fmt.Errorf("missing closing parenthesis")
		}
		return value, nil
	}
	if p.pos >= len(p.runes) || p.runes[p.pos] < 'A' || p.runes[p.pos] > 'J' {
		return false, fmt.Errorf("expected condition letter A-J")
	}
	index := int(p.runes[p.pos] - 'A')
	p.pos++
	if index >= p.count {
		return false, fmt.Errorf("condition %c has no matching condition field", rune('A'+index))
	}
	p.refs[index] = true
	return p.value(index), nil
}

func (p *expressionParser) take(token string) bool {
	runes := []rune(token)
	if len(p.runes)-p.pos < len(runes) {
		return false
	}
	for i, r := range runes {
		if p.runes[p.pos+i] != r {
			return false
		}
	}
	p.pos += len(runes)
	return true
}

func (p *expressionParser) space() {
	for p.pos < len(p.runes) && unicode.IsSpace(p.runes[p.pos]) {
		p.pos++
	}
}
