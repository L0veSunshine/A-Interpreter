package parser

import (
	"Interpreter/ast"
	"Interpreter/errors"
	"Interpreter/lexer"
	"Interpreter/tokens"
	"reflect"
	"strconv"
)

type Parser struct {
	lex *lexer.Lexer
	prevToken,
	curToken,
	peekToken *tokens.Token
	*errors.Errors
	prefixFns map[string]prefixParseFn
	infixFns  map[string]infixParseFn
	SymTable  *SymTable
}

func (p *Parser) regPrefixFn(token string, fn prefixParseFn) {
	p.prefixFns[token] = fn
}

func (p *Parser) regInfixFn(token string, fn infixParseFn) {
	p.infixFns[token] = fn
}

func NewParser(lex *lexer.Lexer) *Parser {
	p := &Parser{
		lex:       lex,
		Errors:    errors.NewErr(),
		prefixFns: map[string]prefixParseFn{},
		infixFns:  map[string]infixParseFn{},
		SymTable:  NewSymTable("Base"),
	}
	//确保词法分析器位置正确
	p.lex.Loc = &tokens.Locate{Column: 1, Line: 1}

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
	p.regPrefixFn(tokens.None, p.parseNone)
	p.regPrefixFn(tokens.If, p.parseIfExpression)
	p.regPrefixFn(tokens.For, p.parseForExpr)
	p.regPrefixFn(tokens.Func, p.parseFuncDef)
	p.regPrefixFn(tokens.LBRACKET, p.parseArray)
	p.regPrefixFn(tokens.LBRACE, p.parseMap)
	p.regPrefixFn(tokens.LF, p.skipLF)

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
	p.regInfixFn(tokens.LBRACKET, p.parseIndexInfix)
	p.regInfixFn(tokens.Dot, p.parseMethodCall)

	p.init()
	p.next()
	return p
}

func (p *Parser) init() {
	p.curToken = p.peekToken
	p.peekToken = p.lex.NextToken()
}

func (p *Parser) next() {
	p.prevToken = p.curToken
	p.curToken = p.peekToken
	p.peekToken = p.lex.NextToken()
}

func (p *Parser) eat(Type ...string) {
	for _, t := range Type {
		if p.curToken.Type == t {
			p.next()
			return
		}
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

func (p *Parser) skipLF() ast.Expression {
	for p.curToken.IsLF() {
		p.next()
	}
	return nil
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
		if p.curToken.Type == tokens.LF {
			p.skipLF()
		} else {
			p.next()
		}
	}
	return ast.Program{Statements: statements}
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case tokens.Var:
		return p.parseVarStatement()
	case tokens.Ident:
		switch p.peekToken.Type {
		case tokens.Assign:
			return p.parseAssignStatement()
		case tokens.LParen:
			return p.parseCallStatement()
		case tokens.LBRACKET:
			return p.parseExprAssign()
		case tokens.Dot:
			return p.parseMethodCallStmt()
		case tokens.IPlus, tokens.IMinus, tokens.IMul,
			tokens.IDiv, tokens.IPow, tokens.IMod:
			return p.parseReplaceAssign()
		}
		return p.parseExprStatement()
	case tokens.Return:
		return p.parseReturnStatement()
	case tokens.Func:
		return p.parseFuncStatement()
	case tokens.Break:
		return p.parseBreakStmt()
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
func (p *Parser) parseBreakStmt() ast.Statement {
	token := p.curToken
	return ast.ExprStatement{Expression: ast.BreakExpr{Token: *token}}
}

func (p *Parser) parseMethodCallStmt() ast.Statement {
	token := p.curToken
	expr := p.parseExpr(LOWEST)
	return ast.MethodCallStmt{
		Token: *token,
		Call:  expr,
	}
}

func (p *Parser) parseReplaceAssign() ast.Statement {
	left := p.parseIdentifier()
	p.next() //skip ident
	op := p.curToken
	p.next() //skip +=,-=,*/...
	expr := p.parseExpr(LOWEST)
	return ast.ExprStatement{Expression: ast.InfixExpr{
		Left:  left,
		Right: expr,
		Op:    *op,
	}}
}

func (p *Parser) parseVarStatement() ast.Statement {
	token := *p.curToken // Var tokens
	p.eatPeek(tokens.Ident)
	ident := ast.IdentNode{
		Token: *p.curToken,
		Value: p.curToken.Literal,
	}
	p.SymTable.Define(p.curToken.Literal, I)
	p.eatPeek(tokens.Assign)
	p.next()
	value := p.parseExpr(LOWEST)
	if p.peekToken.IsLF() {
		p.next()
	}
	if reflect.TypeOf(value).Name() == "MethodCall" {
		return ast.VarMethodCall{
			Token:  token,
			Indent: ident,
			Value:  value,
		}
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
	if p.curToken.Type == tokens.LF {
		return ast.ReturnStatement{
			Token:     token,
			ReturnVal: nil,
		}
	}
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

func (p *Parser) parseCallStatement() ast.Statement {
	fn := p.parseIdentifier()
	p.next()
	token := p.curToken
	args := p.parseExpressionList(tokens.LParen, tokens.RParen)
	exp := ast.FuncCallExpr{Token: *token, Function: fn, Arguments: args}
	p.eat(tokens.RParen)
	return ast.ExprStatement{Expression: exp}
}

func (p *Parser) parseExprAssign() ast.Statement {
	token := p.curToken
	exp := p.parseIdentifier()
	p.next()
	indexExpr := p.parseIndex(exp)
	p.eat(tokens.RBRACKET) // skip ]
	p.eat(tokens.Assign)
	newExp := p.parseExpr(LOWEST)
	p.next()
	return ast.ExpressionAssign{
		Token: *token,
		Old:   indexExpr.Left,
		Key:   indexExpr.Index,
		New:   newExp,
	}
}

func (p *Parser) parseExprStatement() ast.Statement {
	expr := p.parseExpr(LOWEST)
	if p.peekToken.IsLF() {
		p.next()
		p.skipLF()
	}
	return ast.ExprStatement{Expression: expr}
}

func (p *Parser) parseFuncStatement() ast.FuncStatement {
	expr := p.parseExpr(LOWEST)
	return ast.FuncStatement{Expression: expr}
}

func (p *Parser) parseExpr(precedence int) ast.Expression {
	prefix := p.prefixFns[p.curToken.Type]
	if prefix == nil {
		if !p.curToken.IsEOF() {
			p.NewErrorF("no prefix parse function for %s",
				p.curToken.Str())
		}
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

func (p *Parser) parseNone() ast.Expression {
	token := p.curToken
	return ast.NoneNode{Token: *token}
}

func (p *Parser) parseIdentifier() ast.Expression {
	var node ast.Expression
	token := p.curToken
	if p.prevToken.Type == tokens.Dot {
		node = ast.MethodNode{
			Token: *token,
			Value: token.Literal,
		}
		p.SymTable.Methods.Add(token.Literal)
	} else {
		node = ast.IdentNode{
			Token: *token,
			Value: token.Literal,
		}
	}
	return node
}

func (p *Parser) parseOneMethodCall() (
	Methods ast.Expression, Args []ast.Expression) {
	if p.curToken.Type == tokens.LParen {
		p.next()
	}
	p.next() //skip .
	Methods = p.parseIdentifier()
	p.next()
	if p.curToken.Type == tokens.LParen {
		Args = p.parseExpressionList(tokens.LParen, tokens.RParen)
	}
	if p.curToken.Type != tokens.RParen {
		p.NewErrorF("syntax error: Invalid parentheses")
	}
	if p.peekToken.Type == tokens.Dot {
		p.eat(tokens.RParen)
	}
	return
}

func (p *Parser) parseMethodCall(left ast.Expression) ast.Expression {
	token := p.curToken //.
	call := ast.MethodCall{
		Token: *token,
		Left:  left,
	}
	var ms []ast.Expression
	var args [][]ast.Expression
	for p.curToken.Type == tokens.Dot {
		msTmp, argsTmp := p.parseOneMethodCall()
		ms = append(ms, msTmp)
		args = append(args, argsTmp)
	}
	call.Methods = ms
	call.Arguments = args
	if p.peekToken.Type == tokens.LF {
		p.next()
	}
	return call
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
	token := *p.curToken // token if
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
	var init, eachOpt ast.Statement
	var cond ast.Expression
	if p.curToken.Type == tokens.RParen {
		goto Res
	}
	if p.curToken.Type != tokens.Semi {
		init = p.parseStatement()
		p.next()
	}
	if p.curToken.Type == tokens.Semi {
		p.eat(tokens.Semi)
		cond = p.parseExpr(LOWEST)
		p.next()
	}
	if p.curToken.Type == tokens.Semi && p.peekToken.Type != tokens.RParen {
		p.eat(tokens.Semi)
		eachOpt = p.parseStatement()
		p.next()
	} else {
		p.eat(tokens.Semi)
	}
	p.eat(tokens.RParen)
	if !p.find(tokens.LBRACE) {
		p.NewError(`loop body need warped by "{}".`)
	}
Res:
	loop := p.parseBlockStatement()
	return ast.ForExpression{
		Token:       token,
		Condition:   cond,
		InitCond:    init,
		EachOperate: eachOpt,
		Loop:        &loop,
	}
}

func (p *Parser) parseFuncParams() []ast.IdentNode {
	var params []ast.IdentNode
	p.eat(tokens.LParen)
	if p.curToken.Type == tokens.RParen {
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
	p.SymTable.Define(name, F)
	p.SymTable = NewInnerSymTable(name, p.SymTable)
	p.next()
	params := p.parseFuncParams()
	for _, param := range params {
		p.SymTable.Define(param.Value, I)
	}
	body := p.parseBlockStatement()
	p.SymTable = p.SymTable.Outer
	return ast.FuncDef{
		Token:      *token,
		Parameters: params,
		FuncBody:   &body,
		Name:       name,
	}
}

func (p *Parser) parseExpressionList(start, end string) []ast.Expression {
	var args []ast.Expression
	p.eat(start)
	if p.curToken.Type == end {
		return args
	}
	arg := p.parseExpr(LOWEST)
	args = append(args, arg)
	p.next()
	for p.curToken.Type == tokens.Comma {
		p.next()
		arg := p.parseExpr(LOWEST)
		args = append(args, arg)
		p.next()
	}
	return args
}

func (p *Parser) parseCallFunc(function ast.Expression) ast.Expression {
	token := p.curToken // (
	args := p.parseExpressionList(tokens.LParen, tokens.RParen)
	expr := ast.FuncCallExpr{
		Token:     *token,
		Function:  function,
		Arguments: args,
	}
	return expr
}

func (p *Parser) parseArray() ast.Expression {
	token := p.curToken
	args := p.parseExpressionList(tokens.LBRACKET, tokens.RBRACKET)
	return ast.Array{
		Token:    *token,
		Elements: args,
	}
}

func (p *Parser) parseMap() ast.Expression {
	token := p.curToken
	var keys []ast.Expression
	var items []ast.Expression
	p.eat(tokens.LBRACE)
	if p.curToken.Type == tokens.RBRACE {
		return ast.Map{
			Token: *token,
			Keys:  keys,
			Items: items,
		}
	}
	for p.curToken.Type != tokens.RBRACE {
		keys = append(keys, p.parseExpr(LOWEST))
		p.next()
		p.eat(tokens.Colon) //:
		items = append(items, p.parseExpr(LOWEST))
		p.next()
		if p.curToken.Type == tokens.Comma {
			p.eat(tokens.Comma) //skip
		}
	}
	return ast.Map{
		Token: *token,
		Keys:  keys,
		Items: items,
	}
}

func (p *Parser) parseIndexInfix(left ast.Expression) ast.Expression {
	return p.parseIndex(left)
}

func (p *Parser) parseIndex(left ast.Expression) ast.IndexExpression {
	token := p.curToken    // [
	p.eat(tokens.LBRACKET) //skip [
	var arg1 ast.Expression
	if p.curToken.Type != tokens.Colon {
		arg1 = p.parseExpr(LOWEST)
		p.next()
	} else {
		arg1 = nil
	}
	ie := ast.IndexExpression{
		Token: *token,
		Left:  left,
	}
	switch p.curToken.Type {
	case tokens.RBRACKET:
		ie.Index = arg1
	case tokens.Colon:
		if p.peekToken.Type == tokens.RBRACKET {
			slice := ast.IndexSlice{}
			ie.Index = slice
		} else {
			slice := p.parseSlice()
			slice.Start = arg1
			ie.Index = slice
		}
	default:
		p.NewErrorF(`Slice need ":" as break but not %s`, p.curToken.Literal)
	}
	return ie
}

func (p *Parser) parseSlice() ast.IndexSlice {
	slice := ast.IndexSlice{} //current token :
	if p.peekToken.Type != tokens.Colon {
		p.eat(tokens.Colon)
		slice.End = p.parseExpr(LOWEST)
	} else {
		slice.End = nil
	}
	p.next()
	if p.curToken.Type == tokens.RBRACKET {
		return slice
	} else {
		if p.peekToken.Type != tokens.RBRACKET {
			p.eat(tokens.Colon)
			slice.Step = p.parseExpr(LOWEST)
		} else {
			slice.Step = nil
		}
		p.next()
		if p.curToken.Type == tokens.RBRACKET {
			return slice
		}
	}
	return slice
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
