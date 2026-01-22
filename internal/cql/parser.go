package cql

import "fmt"

// Parser converts tokens into an Abstract Syntax Tree
type Parser struct {
	tokens  []Token
	current int
}

// NewParser creates a new QL parser
func NewParser(tokens []Token) *Parser {
	return &Parser{
		tokens:  tokens,
		current: 0,
	}
}

// Error creates a parser error
func (p *Parser) Error(message string) error {
	token := p.peek()
	return fmt.Errorf("QL parse error at token '%s': %s", token.Value, message)
}

// peek returns the current token
func (p *Parser) peek() Token {
	if p.current >= len(p.tokens) {
		return Token{Type: EOF}
	}
	return p.tokens[p.current]
}

// advance moves to the next token
func (p *Parser) advance() Token {
	if p.current < len(p.tokens)-1 {
		p.current++
	}
	return p.tokens[p.current-1]
}

// match checks if the current token matches any of the given types
func (p *Parser) match(types ...TokenType) bool {
	token := p.peek()
	for _, t := range types {
		if token.Type == t {
			return true
		}
	}
	return false
}

// consume advances if the current token matches the expected type
func (p *Parser) consume(tokenType TokenType, message string) (Token, error) {
	if p.peek().Type == tokenType {
		return p.advance(), nil
	}
	return Token{}, p.Error(message)
}

// Parse converts tokens into an AST
func (p *Parser) Parse() (*ASTNode, error) {
	ast, err := p.expression()
	if err != nil {
		return nil, err
	}

	if p.peek().Type != EOF {
		return nil, p.Error("unexpected tokens after expression")
	}

	return ast, nil
}

// expression → orExpression
func (p *Parser) expression() (*ASTNode, error) {
	return p.orExpression()
}

// orExpression → andExpression ( "OR" andExpression )*
func (p *Parser) orExpression() (*ASTNode, error) {
	left, err := p.andExpression()
	if err != nil {
		return nil, err
	}

	for p.match(OR) {
		operator := p.advance()
		right, err := p.andExpression()
		if err != nil {
			return nil, err
		}
		left = &ASTNode{
			Type:     NodeBinaryOp,
			Operator: operator.Value,
			Left:     left,
			Right:    right,
		}
	}

	return left, nil
}

// andExpression → notExpression ( "AND" notExpression )*
func (p *Parser) andExpression() (*ASTNode, error) {
	left, err := p.notExpression()
	if err != nil {
		return nil, err
	}

	for p.match(AND) {
		operator := p.advance()
		right, err := p.notExpression()
		if err != nil {
			return nil, err
		}
		left = &ASTNode{
			Type:     NodeBinaryOp,
			Operator: operator.Value,
			Left:     left,
			Right:    right,
		}
	}

	return left, nil
}

// notExpression → "NOT" comparison | comparison
func (p *Parser) notExpression() (*ASTNode, error) {
	if p.match(NOT) {
		operator := p.advance()
		operand, err := p.comparison()
		if err != nil {
			return nil, err
		}
		return &ASTNode{
			Type:     NodeBinaryOp,
			Operator: operator.Value,
			Right:    operand,
		}, nil
	}

	return p.comparison()
}

// comparison → primary ( operator primary )*
func (p *Parser) comparison() (*ASTNode, error) {
	left, err := p.primary()
	if err != nil {
		return nil, err
	}

	if p.match(EQUALS, NOT_EQUALS, LESS_THAN, LESS_EQUAL, GREATER_THAN, GREATER_EQUAL, CONTAINS) {
		operator := p.advance()
		right, err := p.primary()
		if err != nil {
			return nil, err
		}
		return &ASTNode{
			Type:     NodeComparison,
			Operator: operator.Value,
			Left:     left,
			Right:    right,
		}, nil
	}

	if p.match(IN, NOT_IN) {
		operator := p.advance()
		_, err := p.consume(LPAREN, "expected ( after IN")
		if err != nil {
			return nil, err
		}
		values, err := p.valueList()
		if err != nil {
			return nil, err
		}
		_, err = p.consume(RPAREN, "expected ) after IN values")
		if err != nil {
			return nil, err
		}
		return &ASTNode{
			Type:     NodeInExpression,
			Operator: operator.Value,
			Field:    left,
			Values:   values,
		}, nil
	}

	return left, nil
}

// primary → identifier | literal | function | "(" expression ")"
func (p *Parser) primary() (*ASTNode, error) {
	if p.match(IDENTIFIER) {
		token := p.advance()
		return &ASTNode{
			Type:  NodeIdentifier,
			Value: token.Value,
		}, nil
	}

	if p.match(STRING, NUMBER, DATE, BOOLEAN) {
		token := p.advance()
		return &ASTNode{
			Type:     NodeLiteral,
			DataType: token.Type,
			Value:    token.Value,
		}, nil
	}

	if p.match(FUNCTION) {
		token := p.advance()
		_, err := p.consume(LPAREN, "expected ( after function name")
		if err != nil {
			return nil, err
		}

		var args []*ASTNode
		if !p.match(RPAREN) {
			arg, err := p.expression()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)

			for p.match(COMMA) {
				p.advance()
				arg, err := p.expression()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
			}
		}

		_, err = p.consume(RPAREN, "expected ) after function arguments")
		if err != nil {
			return nil, err
		}

		return &ASTNode{
			Type:      NodeFunction,
			Value:     token.Value,
			Arguments: args,
		}, nil
	}

	if p.match(LPAREN) {
		p.advance()
		expr, err := p.expression()
		if err != nil {
			return nil, err
		}
		_, err = p.consume(RPAREN, "expected )")
		if err != nil {
			return nil, err
		}
		return expr, nil
	}

	return nil, p.Error("expected identifier, literal, function, or (")
}

// valueList → value ( "," value )*
func (p *Parser) valueList() (*ASTNode, error) {
	var values []*ASTNode

	if p.match(STRING, NUMBER, DATE, IDENTIFIER) {
		token := p.advance()
		values = append(values, &ASTNode{
			Type:     NodeLiteral,
			DataType: token.Type,
			Value:    token.Value,
		})

		for p.match(COMMA) {
			p.advance()
			if p.match(STRING, NUMBER, DATE, IDENTIFIER) {
				token := p.advance()
				values = append(values, &ASTNode{
					Type:     NodeLiteral,
					DataType: token.Type,
					Value:    token.Value,
				})
			} else {
				return nil, p.Error("expected value after comma")
			}
		}
	}

	return &ASTNode{
		Type:      NodeList,
		Arguments: values,
	}, nil
}
