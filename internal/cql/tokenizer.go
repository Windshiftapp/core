package cql

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Tokenizer converts QL query strings into tokens
type Tokenizer struct {
	input    string
	position int
	current  rune
}

// NewTokenizer creates a new QL tokenizer
func NewTokenizer(input string) *Tokenizer {
	t := &Tokenizer{
		input:    input,
		position: 0,
	}
	if len(input) > 0 {
		t.current = rune(input[0])
	}
	return t
}

// Error creates a tokenizer error with position information
func (t *Tokenizer) Error(message string) error {
	return fmt.Errorf("QL syntax error at position %d: %s", t.position, message)
}

// advance moves to the next character
func (t *Tokenizer) advance() {
	t.position++
	if t.position >= len(t.input) {
		t.current = 0 // EOF
	} else {
		t.current = rune(t.input[t.position])
	}
}

// skipWhitespace skips whitespace characters
func (t *Tokenizer) skipWhitespace() {
	for t.current != 0 && unicode.IsSpace(t.current) {
		t.advance()
	}
}

// readString reads a quoted string
func (t *Tokenizer) readString() (string, error) {
	quote := t.current
	var value strings.Builder
	t.advance()

	for t.current != 0 && t.current != quote {
		if t.current == '\\' {
			t.advance()
			if t.current != 0 {
				value.WriteRune(t.current)
				t.advance()
			}
		} else {
			value.WriteRune(t.current)
			t.advance()
		}
	}

	if t.current == 0 {
		return "", t.Error("unterminated string literal")
	}

	t.advance() // Skip closing quote
	return value.String(), nil
}

// readNumber reads a numeric value
func (t *Tokenizer) readNumber() string {
	var value strings.Builder
	for t.current != 0 && (unicode.IsDigit(t.current) || t.current == '.') {
		value.WriteRune(t.current)
		t.advance()
	}
	return value.String()
}

// readIdentifier reads an identifier or keyword
func (t *Tokenizer) readIdentifier() string {
	var value strings.Builder
	for t.current != 0 && (unicode.IsLetter(t.current) || unicode.IsDigit(t.current) || t.current == '_' || t.current == '-') {
		value.WriteRune(t.current)
		t.advance()
	}
	return value.String()
}

// peekAhead looks ahead in the input without advancing position
func (t *Tokenizer) peekAhead(offset int) rune {
	pos := t.position + offset
	if pos >= len(t.input) {
		return 0
	}
	return rune(t.input[pos])
}

// isDatePattern checks if the current position looks like a date (YYYY-MM-DD)
func (t *Tokenizer) isDatePattern() bool {
	// Check for YYYY-MM-DD pattern
	if t.position+9 >= len(t.input) {
		return false
	}
	pattern := t.input[t.position : t.position+10]
	matched, err := regexp.MatchString(`\d{4}-\d{2}-\d{2}`, pattern)
	if err != nil {
		return false
	}
	return matched
}

// Tokenize converts the input string into tokens
func (t *Tokenizer) Tokenize() ([]Token, error) {
	var tokens []Token

	for t.current != 0 {
		t.skipWhitespace()

		if t.current == 0 {
			break
		}

		start := t.position

		// String literals
		if t.current == '"' || t.current == '\'' {
			value, err := t.readString()
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, Token{Type: STRING, Value: value, Pos: start})
			continue
		}

		// Numbers and dates
		if unicode.IsDigit(t.current) {
			if t.isDatePattern() {
				// Read as date
				date := t.input[t.position : t.position+10]
				for i := 0; i < 10; i++ {
					t.advance()
				}
				tokens = append(tokens, Token{Type: DATE, Value: date, Pos: start})
			} else {
				// Read as number
				number := t.readNumber()
				tokens = append(tokens, Token{Type: NUMBER, Value: number, Pos: start})
			}
			continue
		}

		// Identifiers and keywords
		if unicode.IsLetter(t.current) || t.current == '_' {
			identifier := t.readIdentifier()
			upper := strings.ToUpper(identifier)

			switch upper {
			case "AND":
				tokens = append(tokens, Token{Type: AND, Value: "AND", Pos: start})
			case "OR":
				tokens = append(tokens, Token{Type: OR, Value: "OR", Pos: start})
			case "NOT":
				// Look ahead for "NOT IN"
				oldPos := t.position
				t.skipWhitespace()
				if t.position+1 < len(t.input) && strings.ToUpper(t.input[t.position:t.position+2]) == "IN" {
					t.advance()
					t.advance()
					tokens = append(tokens, Token{Type: NOT_IN, Value: "NOT IN", Pos: start})
				} else {
					t.position = oldPos
					t.current = rune(t.input[t.position])
					tokens = append(tokens, Token{Type: NOT, Value: "NOT", Pos: start})
				}
			case "IN":
				tokens = append(tokens, Token{Type: IN, Value: "IN", Pos: start})
			case "TRUE", "FALSE":
				tokens = append(tokens, Token{Type: BOOLEAN, Value: strings.ToLower(identifier), Pos: start})
			default:
				// Check if it's a function (followed by parentheses)
				oldPos := t.position
				t.skipWhitespace()
				if t.current == '(' {
					tokens = append(tokens, Token{Type: FUNCTION, Value: identifier, Pos: start})
				} else {
					tokens = append(tokens, Token{Type: IDENTIFIER, Value: identifier, Pos: start})
				}
				t.position = oldPos
				if t.position < len(t.input) {
					t.current = rune(t.input[t.position])
				} else {
					t.current = 0
				}
			}
			continue
		}

		// Two-character operators
		if t.current == '!' && t.peekAhead(1) == '=' {
			t.advance()
			t.advance()
			tokens = append(tokens, Token{Type: NOT_EQUALS, Value: "!=", Pos: start})
			continue
		}

		if t.current == '<' && t.peekAhead(1) == '=' {
			t.advance()
			t.advance()
			tokens = append(tokens, Token{Type: LESS_EQUAL, Value: "<=", Pos: start})
			continue
		}

		if t.current == '>' && t.peekAhead(1) == '=' {
			t.advance()
			t.advance()
			tokens = append(tokens, Token{Type: GREATER_EQUAL, Value: ">=", Pos: start})
			continue
		}

		if t.current == '<' && t.peekAhead(1) == '>' {
			t.advance()
			t.advance()
			tokens = append(tokens, Token{Type: NOT_EQUALS, Value: "<>", Pos: start})
			continue
		}

		// Single-character tokens
		switch t.current {
		case '=':
			tokens = append(tokens, Token{Type: EQUALS, Value: "=", Pos: start})
		case '<':
			tokens = append(tokens, Token{Type: LESS_THAN, Value: "<", Pos: start})
		case '>':
			tokens = append(tokens, Token{Type: GREATER_THAN, Value: ">", Pos: start})
		case '~':
			tokens = append(tokens, Token{Type: CONTAINS, Value: "~", Pos: start})
		case '(':
			tokens = append(tokens, Token{Type: LPAREN, Value: "(", Pos: start})
		case ')':
			tokens = append(tokens, Token{Type: RPAREN, Value: ")", Pos: start})
		case ',':
			tokens = append(tokens, Token{Type: COMMA, Value: ",", Pos: start})
		default:
			return nil, t.Error(fmt.Sprintf("unexpected character: %c", t.current))
		}
		t.advance()
	}

	tokens = append(tokens, Token{Type: EOF, Value: "", Pos: t.position})
	return tokens, nil
}
