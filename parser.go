package main

import (
	"Interpreter/ast"
	"Interpreter/tokens"
	"fmt"
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
	p.regPrefixFn(tokens.Not, p.parsePrefixExpr)
	p.regPrefixFn(tokens.Int, p.parseInt)
	p.regPrefixFn(tokens.Float, p.parseFloat)
	p.regPrefixFn(tokens.Ident, p.parseIdentifier)
	p.regPrefixFn(tokens.String, p.parseString)
	p.regPrefixFn(tokens.False, p.parseBoolean)
	p.regPrefixFn(tokens.True, p.parseBoolean)
	p.regPrefixFn(tokens.If, p.parseIfExpression)
	p.regPrefixFn(tokens.For, p.parseForExpr)
	p.regPrefixFn(tokens.Func, p.parseFuncDef)

	p.regInfixFn(tokens.Minus, p.parseInfixExpr)
	p.regInfixFn(tokens.Plus, p.parseInfixExpr)
	p.regInfixFn(tokens.Mul, p.parseInfixExpr)
	p.regInfixFn(tokens.Div, p.parseInfixExpr)
	p.regInfixFn(tokens.Mod, p.parseInfixExpr)
	p.regInfixFn(tokens.Pow, p.parseInfixExpr)
	p.regInfixFn(tokens.LT, p.parseInfixExpr)
	p.regInfixFn(tokens.LTEq, p.parseInfixExpr)
	p.regInfixFn(tokens.GT, p.parseInfixExpr)
	p.regInfixFn(tokens.GTEq, p.parseInfixExpr)
	p.regInfixFn(tokens.Equal, p.parseInfixExpr)
	p.regInfixFn(tokens.NotEq, p.parseInfixExpr)
	p.regInfixFn(tokens.And, p.parseInfixExpr)
	p.regInfixFn(tokens.Or, p.parseInfixExpr)

	p.regInfixFn(tokens.LParen, p.parseCallFunc)

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

func (p *Parser) find(Type string) bool {
	for p.curToken.Type != Type && !p.curToken.IsEOF() && p.curToken.IsLF() {
		p.next()
	}
	return p.curToken.Type == Type
}

func (p *Parser) skipLF() {
	for p.curToken.IsLF() {
		p.next()
	}
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
	for !p.curToken.IsEOF() {
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
		if p.peekToken.Type == tokens.Assign {
			return p.parseAssignStatement()
		}
		return p.parseExprStatement()
	case tokens.Return:
		return p.parseReturnStatement()
	case tokens.LF:
		p.next()
		return p.parseStatement()
	default:
		res := p.parseExprStatement()
		if p.peekToken.IsLF() {
			p.next()
		}
		return res
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
	if p.peekToken.IsLF() {
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
	if p.peekToken.IsLF() {
		p.next()
	}
	return ast.ReturnStatement{
		Token:     token,
		ReturnVal: returnVal,
	}
}

func (p *Parser) parseAssignStatement() ast.Statement {
	Ident := *p.curToken // Ident token
	identifier := p.parseIdentifier()
	p.eat(tokens.Ident)
	p.eat(tokens.Assign)
	stmt := p.parseExpr(LOWEST)
	if p.peekToken.IsLF() {
		p.next()
	}
	return ast.AssignStatement{
		Ident:      Ident,
		Identifier: identifier.(ast.IdentNode),
		Statement:  stmt,
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
		p.NewErrorF("no prefix parse function for %s",
			p.curToken.Str())
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
	var priority = PREFIX
	if token.Type == tokens.Not {
		priority = COMPARE
	}
	right := p.parseExpr(priority)
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

func (p *Parser) parseInt() ast.Expression {
	var token = *p.curToken
	IntVal, e := strconv.ParseInt(p.curToken.Literal, 10, 64)
	if e != nil {
		p.Push(e)
		return nil
	}
	return ast.IntNode{
		Token: token,
		Value: int(IntVal),
	}
}

func (p *Parser) parseFloat() ast.Expression {
	var token = *p.curToken
	floatVal, e := strconv.ParseFloat(p.curToken.Literal, 64)
	if e != nil {
		p.Push(e)
		return nil
	}
	return ast.FloatNode{
		Token: token,
		Value: floatVal,
	}
}

func (p *Parser) parseIdentifier() ast.Expression {
	node := ast.IdentNode{
		Token: *p.curToken,
		Value: p.curToken.Literal,
	}
	return node
}

func (p *Parser) parseString() ast.Expression {
	return ast.StringNode{
		Token: *p.curToken,
		Value: p.curToken.Literal,
	}
}

func (p *Parser) parseBoolean() ast.Expression {
	boolVal, err := strconv.ParseBool(p.curToken.Literal)
	if err != nil {
		p.Push(err)
	}
	return ast.BooleanNode{
		Token: *p.curToken,
		Value: boolVal,
	}
}

func (p *Parser) parseBlockStatement() ast.BlockStatement {
	token := *p.curToken
	p.eat(tokens.LBRACE)
	p.skipLF()
	var s []ast.Statement
	for p.curToken.Type != tokens.RBRACE && !p.curToken.IsEOF() {
		stmt := p.parseStatement()
		if stmt != nil {
			s = append(s, stmt)
		}
		p.next() //skip }
		p.skipLF()
	}
	p.eat(tokens.RBRACE)
	return ast.BlockStatement{
		Token:      token,
		Statements: s,
	}
}

func (p *Parser) parseIfExpression() ast.Expression {
	token := *p.curToken //if
	p.eat(tokens.If)
	p.eat(tokens.LParen)
	cond := p.parseExpr(LOWEST)
	p.next()
	p.eat(tokens.RParen)
	if !p.find(tokens.LBRACE) {
		p.NewError(`condition need warped by "{}".`)
	}
	conSeq := p.parseBlockStatement()
	if p.curToken.Type == tokens.Else {
		p.eat(tokens.Else)
		alter := p.parseBlockStatement()
		return ast.IfExpression{
			Token:       token,
			Condition:   cond,
			Consequence: &conSeq,
			Alternative: &alter,
		}
	} else {
		return ast.IfExpression{
			Token:       token,
			Condition:   cond,
			Consequence: &conSeq,
		}
	}
}

func (p *Parser) parseForExpr() ast.Expression {
	token := *p.curToken //token 'for'
	p.eat(tokens.For)
	p.eat(tokens.LParen)
	cond := p.parseExpr(LOWEST)
	p.next()
	p.eat(tokens.RParen)
	if !p.find(tokens.LBRACE) {
		p.NewError(`loop need warped by "{}".`)
	}
	loop := p.parseBlockStatement()
	return ast.ForExpression{
		Token:     token,
		Condition: cond,
		Loop:      &loop,
	}
}

func (p *Parser) parseFuncParams() []ast.IdentNode {
	var params []ast.IdentNode
	p.eat(tokens.LParen)
	if p.peekToken.Type == tokens.RParen {
		p.next()
		return params
	}
	param := ast.IdentNode{
		Token: *p.curToken,
		Value: p.curToken.Literal,
	}
	p.next()
	params = append(params, param)
	for p.curToken.Type == tokens.Comma {
		p.next() //skip comma
		param := ast.IdentNode{
			Token: *p.curToken,
			Value: p.curToken.Literal,
		}
		params = append(params, param)
		p.next()
	}
	p.eat(tokens.RParen)
	return params
}

func (p *Parser) parseFuncDef() ast.Expression {
	token := p.curToken
	p.eat(tokens.Func)
	name := p.curToken.Literal
	p.next()
	params := p.parseFuncParams()
	fmt.Println(p.curToken)
	body := p.parseBlockStatement()
	return ast.FuncDef{
		Token:      *token,
		Parameters: params,
		FuncBody:   &body,
		Name:       name,
	}
}

func (p *Parser) parseCallArgs() []ast.Expression {
	var args []ast.Expression
	p.eat(tokens.LParen)
	if p.peekToken.Type == tokens.RParen {
		p.eat(tokens.RParen)
		return args
	}
	arg := p.parseExpr(LOWEST)
	p.next()
	args = append(args, arg)
	for p.curToken.Type == tokens.Comma {
		p.next()
		arg := p.parseExpr(LOWEST)
		args = append(args, arg)
		p.next()
	}
	return args
}

func (p *Parser) parseCallFunc(function ast.Expression) ast.Expression {
	token := p.curToken
	args := p.parseCallArgs()
	expr := ast.FuncCallExpr{
		Token:     *token,
		Function:  function,
		Arguments: args,
	}
	return expr
}

func (p *Parser) parseGroupedExpr() ast.Expression {
	p.eat(tokens.LParen)
	exp := p.parseExpr(LOWEST)
	p.eatPeek(tokens.RParen)
	return exp
}

func (p *Parser) Parse() ast.Statement {
	return p.parseProgram()
}
