package main

import (
	"Interpreter/ast"
	"Interpreter/tokens"
	"strconv"
)

type Parser struct {
	lex *Lexer
	curToken,
	peekToken *tokens.Token
	*Errors
	prefixFns map[string]prefixParseFn
	infixFns  map[string]infixParseFn
}

func (p *Parser) regPrefixFn(token string, fn prefixParseFn) {
	p.prefixFns[token] = fn
}

func (p *Parser) regInfixFn(token string, fn infixParseFn) {
	p.infixFns[token] = fn
}

func NewParser(lex *Lexer) *Parser {
	p := &Parser{
		lex:       lex,
		Errors:    NewErr(),
		prefixFns: map[string]prefixParseFn{},
		infixFns:  map[string]infixParseFn{},
	}
	//确保词法分析器位置正确
	p.lex.loc = &tokens.Locate{Column: 1, Line: 1}

	p.regPrefixFn(tokens.LParen, p.parseGroupedExpr)
	p.regPrefixFn(tokens.Plus, p.parsePrefixExpr)
	p.regPrefixFn(tokens.Minus, p.parsePrefixExpr)
	p.regPrefixFn(tokens.Number, p.parseNumber)
	p.regPrefixFn(tokens.Ident, p.parseIdentifier)
	p.regPrefixFn(tokens.String, p.parseString)
	p.regPrefixFn(tokens.False, p.parseBoolean)
	p.regPrefixFn(tokens.True, p.parseBoolean)

	p.regInfixFn(tokens.Minus, p.parseInfixExpr)
	p.regInfixFn(tokens.Plus, p.parseInfixExpr)
	p.regInfixFn(tokens.Mul, p.parseInfixExpr)
	p.regInfixFn(tokens.Div, p.parseInfixExpr)
	p.regInfixFn(tokens.Floor, p.parseInfixExpr)
	p.regInfixFn(tokens.Pow, p.parseInfixExpr)

	p.next()
	p.next()
	return p
}

func (p *Parser) next() {
	p.curToken = p.peekToken
	p.peekToken = p.lex.NextToken()
}

func (p *Parser) eat(Type string) {
	if p.curToken.Type == Type {
		p.next()
		return
	}
	p.NewErrorF("Want %s but get %s.(col%d,line%d)", Type, p.curToken.Type,
		p.curToken.Loc.Column, p.curToken.Loc.Line)
}
func (p *Parser) eatPeek(Type string) bool {
	if p.peekToken.Type == Type {
		p.next()
		return true
	}
	p.NewErrorF("Want %s but get %s.(col%d,line%d)", Type, p.curToken.Type,
		p.curToken.Loc.Column, p.curToken.Loc.Line)
	return false
}

func (p *Parser) parseProgram() ast.Program {
	var statements []ast.Statement
	for p.curToken.Type != tokens.EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			statements = append(statements, stmt)
		}
		p.next()
	}
	return ast.Program{Statements: statements}
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case tokens.Var:
		return p.parseVarStatement()
	case tokens.Ident:
		return p.parseAssignStatement()
	case tokens.Return:
		return p.parseReturnStatement()
	default:
		return p.parseExprStatement()
	}
}

func (p *Parser) parseVarStatement() ast.Statement {
	token := *p.curToken // Var tokens
	p.eatPeek(tokens.Ident)
	ident := ast.IdentNode{
		Token: *p.curToken,
		Value: p.curToken.Literal,
	}
	p.eatPeek(tokens.Assign)
	p.next()
	value := p.parseExpr(LOWEST)
	if p.peekToken.Type == tokens.LF {
		p.next()
	}
	return ast.VarStatement{
		Token:  token,
		Indent: ident,
		Value:  value,
	}
}

func (p *Parser) parseReturnStatement() ast.Statement {
	token := *p.curToken // Return tokens
	p.eat(tokens.Return)
	returnVal := p.parseExpr(LOWEST)
	if p.peekToken.Type == tokens.LF {
		p.next()
	}
	return ast.ReturnStatement{
		Token:     token,
		ReturnVal: returnVal,
	}
}

func (p *Parser) parseAssignStatement() ast.Statement {
	Ident := *p.curToken // Ident token
	p.eat(tokens.Ident)
	p.eat(tokens.Assign)
	stmt := p.parseExpr(LOWEST)
	if p.peekToken.Type == tokens.LF {
		p.next()
	}
	return ast.AssignStatement{
		Ident:     Ident,
		Statement: stmt,
	}
}

func (p *Parser) parseExprStatement() ast.Statement {
	expr := p.parseExpr(LOWEST)
	return ast.ExprStatement{
		Expression: expr,
	}
}

func (p *Parser) parseExpr(precedence int) ast.Expression {
	prefix := p.prefixFns[p.curToken.Type]
	if prefix == nil {
		p.NewErrorF("no prefix parse function for %s found",
			p.curToken.Type)
		return nil
	}
	left := prefix()
	for precedence < p.peekPrecedence() {
		infix := p.infixFns[p.peekToken.Type]
		if infix == nil {
			return left
		}
		p.next()
		left = infix(left)
	}
	return left
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) parsePrefixExpr() ast.Expression {
	token := *p.curToken
	p.next()
	right := p.parseExpr(PREFIX)
	return ast.PrefixExpr{
		Op:    token,
		Right: right,
	}
}

func (p *Parser) parseInfixExpr(left ast.Expression) ast.Expression {
	op := *p.curToken
	precedence := p.curPrecedence()
	p.next()
	right := p.parseExpr(precedence)
	return ast.InfixExpr{
		Left:  left,
		Right: right,
		Op:    op,
	}
}

func (p *Parser) parseNumber() ast.Expression {
	var token = *p.curToken
	floatVal, e := strconv.ParseFloat(p.curToken.Literal, 64)
	if e != nil {
		p.Push(e)
		return nil
	}
	return ast.NumberNode{
		Token: token,
		Value: floatVal,
	}
}

func (p *Parser) parseIdentifier() ast.Expression {
	return ast.IdentNode{
		Token: *p.curToken,
		Value: p.curToken.Literal,
	}
}

func (p *Parser) parseString() ast.Expression {
	return ast.StringNode{
		Token: *p.curToken,
		Value: p.curToken.Literal,
	}
}

func (p *Parser) parseBoolean() ast.Expression {
	return ast.BooleanNode{
		Token: *p.curToken,
		Value: p.curToken.Literal,
	}
}

//func (p *Parser) factor() Node {
//	tokens := p.curToken
//	switch tokens.Type {
//	case Plus:
//		p.eat(Plus)
//		return PrefixExpr{
//			Right: p.factor(),
//			Op:    *tokens,
//		}
//	case Minus:
//		p.eat(Minus)
//		return PrefixExpr{
//			Right: p.factor(),
//			Op:    *tokens,
//		}
//	case LParen:
//		p.eat(LParen)
//		val := p.expr()
//		p.eat(RParen)
//		return val
//	case Number:
//		floatVal, e := strconv.ParseFloat(p.curToken.Literal, 64)
//		var tokens = *p.curToken
//		if e != nil {
//			p.Push(e)
//			return nil
//		}
//		p.eat(Number)
//		return NumberNode{
//			Token: tokens,
//			Value: floatVal,
//		}
//	default:
//		return nil
//	}
//}

//func (p *Parser) term() Node {
//	var node Node
//	node = p.midFactor()
//	for p.curToken.Type == Mul || p.curToken.Type == Div ||
//		p.curToken.Type == Floor {
//		tokens := *p.curToken
//		if p.curToken.Type == Mul {
//			p.eat(Mul)
//		} else if p.curToken.Type == Floor {
//			p.eat(Floor)
//		} else if p.curToken.Type == Div {
//			p.eat(Div)
//		}
//		node = InfixExpr{
//			Left:  node,
//			Right: p.midFactor(),
//			Op:    tokens,
//		}
//	}
//	return node
//}

//func (p *Parser) midFactor() Node {
//	var node Node
//	node = p.factor()
//	for p.curToken.Type == Pow {
//		tokens := *p.curToken
//		if p.curToken.Type == Pow {
//			p.eat(Pow)
//		}
//		node = InfixExpr{
//			Left:  node,
//			Right: p.factor(),
//			Op:    tokens,
//		}
//	}
//	return node
//}

//func (p *Parser) expr() Node {
//	var node Node
//	node = p.term()
//	for p.curToken.Type == Plus || p.curToken.Type == Minus {
//		tokens := *p.curToken
//		if p.curToken.Type == Plus {
//			p.eat(Plus)
//		} else if p.curToken.Type == Minus {
//			p.eat(Minus)
//		}
//		node = InfixExpr{
//			Left:  node,
//			Op:    tokens,
//			Right: p.term(),
//		}
//	}
//	return node
//}

func (p *Parser) parseGroupedExpr() ast.Expression {
	p.eat(tokens.LParen)
	exp := p.parseExpr(LOWEST)
	p.eatPeek(tokens.RParen)
	return exp
}

func (p *Parser) Parse() ast.Statement {
	return p.parseProgram()
}
