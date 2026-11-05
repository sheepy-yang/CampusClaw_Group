package mcp

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"
)

type expressionParser struct {
	input string
	pos   int
}

func parseNumericExpression(input string) (float64, error) {
	parser := &expressionParser{input: normalizedExpression(input)}
	if parser.input == "" {
		return 0, errors.New("empty expression")
	}
	value, err := parser.parseExpression()
	if err != nil {
		return 0, err
	}
	if parser.pos != len(parser.input) || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("unsupported expression")
	}
	return value, nil
}

func (p *expressionParser) parseExpression() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.input) {
		operator := p.input[p.pos]
		if operator != '+' && operator != '-' {
			break
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if operator == '+' {
			left += right
		} else {
			left -= right
		}
	}
	return left, nil
}

func (p *expressionParser) parseTerm() (float64, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.input) {
		operator := p.input[p.pos]
		if operator != '*' && operator != '/' {
			break
		}
		p.pos++
		right, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		if operator == '*' {
			left *= right
		} else {
			if right == 0 {
				return 0, errors.New("division by zero")
			}
			left /= right
		}
	}
	return left, nil
}

func (p *expressionParser) parsePower() (float64, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return 0, err
	}
	if p.pos < len(p.input) && p.input[p.pos] == '^' {
		p.pos++
		right, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		left = math.Pow(left, right)
	}
	if math.IsNaN(left) || math.IsInf(left, 0) {
		return 0, errors.New("numeric result is not finite")
	}
	return left, nil
}

func (p *expressionParser) parseUnary() (float64, error) {
	if p.pos < len(p.input) && (p.input[p.pos] == '+' || p.input[p.pos] == '-') {
		operator := p.input[p.pos]
		p.pos++
		value, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		if operator == '-' {
			return -value, nil
		}
		return value, nil
	}
	return p.parsePower()
}

func (p *expressionParser) parsePrimary() (float64, error) {
	if p.pos >= len(p.input) {
		return 0, errors.New("missing operand")
	}
	if p.input[p.pos] == '(' {
		p.pos++
		value, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return 0, errors.New("unclosed parenthesis")
		}
		p.pos++
		return value, nil
	}

	start := p.pos
	hasDigit := false
	for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
		hasDigit = true
		p.pos++
	}
	if p.pos < len(p.input) && p.input[p.pos] == '.' {
		p.pos++
		for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
			hasDigit = true
			p.pos++
		}
	}
	if !hasDigit {
		return 0, errors.New("unsupported operand")
	}
	if p.pos < len(p.input) && (p.input[p.pos] == 'e' || p.input[p.pos] == 'E') {
		exponentStart := p.pos
		p.pos++
		if p.pos < len(p.input) && (p.input[p.pos] == '+' || p.input[p.pos] == '-') {
			p.pos++
		}
		digits := p.pos
		for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
			p.pos++
		}
		if digits == p.pos {
			p.pos = exponentStart
		}
	}
	return strconv.ParseFloat(p.input[start:p.pos], 64)
}

func normalizedExpression(input string) string {
	replacer := strings.NewReplacer(
		"×", "*",
		"·", "*",
		"÷", "/",
		"−", "-",
		"（", "(",
		"）", ")",
	)
	input = replacer.Replace(strings.TrimSpace(input))
	return strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) {
			return -1
		}
		return unicode.ToLower(character)
	}, input)
}
