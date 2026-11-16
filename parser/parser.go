// Package parser 实现手写递归下降 + Pratt 表达式解析。
//
// 全文件遵守同一套游标约定，避免"谁该 advance"这类隐性契约：
//
//  1. cur() 是"下一个待消费的 token"，advance() 消费它；
//     peek() 只看下一个，token(k) 提供 k 个 token 的前瞻。
//  2. 每个 parseXxx 消费掉自己负责的 token，返回时 cur 指向构造之后的位置。
//  3. 语句之间的分隔符（换行/分号）只由 endStatement() 处理，
//     parseXxx 自己不吞换行，因此不会吃掉外层块的右花括号。
//  4. 容器（实参表、数组、map、下标）内部允许换行，由 skipNewlines() 处理。
//  5. 所有错误都带 line/col；语句级出错后用 synchronize() 恢复到下一条语句。
//
// 表达式用 Pratt（优先级爬升）解析：新增运算符只需在 precedences 和
// prefixFns/infixFns 里登记，不必改动 parseExpr 的分支。
package parser

import (
	"Interpreter/ast"
	"Interpreter/errors"
	"Interpreter/lexer"
	"Interpreter/tokens"
	"fmt"
	"strconv"
)

// maxErrors 限制错误数量：语法错误级联时不能让错误切片无限增长。
const maxErrors = 100

type Parser struct {
	lex *lexer.Lexer
	// buf 是 token 前瞻缓冲：buf[0] 是当前 token，buf[1] 是 peek，依此类推。
	buf []*tokens.Token
	// tokenSeq 记录 advance() 的调用次数，用于检测解析循环是否停滞
	tokenSeq int
	// scopeSeq 给每个作用域编号并写进 AST，
	// 让编译器遍历 AST 时能找回解析期建立的那张符号表
	scopeSeq int

	*errors.Errors
	prefixFns map[string]prefixParseFn
	infixFns  map[string]infixParseFn
	SymTable  *SymTable
}

func NewParser(lex *lexer.Lexer) *Parser {
	p := &Parser{
		lex:       lex,
		Errors:    errors.NewErr(),
		prefixFns: map[string]prefixParseFn{},
		infixFns:  map[string]infixParseFn{},
		SymTable:  NewSymTable("Base"),
	}

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

	p.regInfixFn(tokens.Minus, p.parseInfixExpr)
	p.regInfixFn(tokens.Plus, p.parseInfixExpr)
	p.regInfixFn(tokens.Mul, p.parseInfixExpr)
	p.regInfixFn(tokens.Div, p.parseInfixExpr)
	p.regInfixFn(tokens.Mod, p.parseInfixExpr)
	p.regInfixFn(tokens.FloorDiv, p.parseInfixExpr)
	p.regInfixFn(tokens.Pow, p.parseInfixExpr)
	p.regInfixFn(tokens.LT, p.parseInfixExpr)
	p.regInfixFn(tokens.LTEq, p.parseInfixExpr)
	p.regInfixFn(tokens.GT, p.parseInfixExpr)
	p.regInfixFn(tokens.GTEq, p.parseInfixExpr)
	p.regInfixFn(tokens.Equal, p.parseInfixExpr)
	p.regInfixFn(tokens.NotEq, p.parseInfixExpr)
	p.regInfixFn(tokens.And, p.parseInfixExpr)
	p.regInfixFn(tokens.Or, p.parseInfixExpr)

	p.regInfixFn(tokens.LParen, p.parseCallExpr)
	p.regInfixFn(tokens.LBRACKET, p.parseIndexExpr)
	p.regInfixFn(tokens.Dot, p.parseMethodCall)

	return p
}

func (p *Parser) regPrefixFn(token string, fn prefixParseFn) { p.prefixFns[token] = fn }
func (p *Parser) regInfixFn(token string, fn infixParseFn)   { p.infixFns[token] = fn }

// ---------------------------------------------------------------------------
// token 游标
// ---------------------------------------------------------------------------

// token 返回第 k 个 token（0 = 当前）。按需从词法分析器补充缓冲。
// EOF 之后词法分析器一直返回 EOF，缓冲不会无限增长（advance 每次都会裁剪）。
func (p *Parser) token(k int) *tokens.Token {
	for len(p.buf) <= k {
		p.buf = append(p.buf, p.lex.NextToken())
	}
	return p.buf[k]
}

func (p *Parser) cur() *tokens.Token { return p.token(0) }

// peek 返回下一个 token。
func (p *Parser) peek() *tokens.Token { return p.token(1) }

func (p *Parser) advance() *tokens.Token {
	tok := p.token(0)
	p.token(1) // 保证 peek 有效
	p.buf = p.buf[1:]
	p.tokenSeq++
	return tok
}

// at 判断当前 token 类型是否属于给定集合。
func (p *Parser) at(types ...string) bool {
	return matchType(p.cur(), types)
}

// peekAt 判断下一个 token 类型是否属于给定集合。
func (p *Parser) peekAt(types ...string) bool {
	return matchType(p.peek(), types)
}

func matchType(tok *tokens.Token, types []string) bool {
	for _, t := range types {
		if tok.Type == t {
			return true
		}
	}
	return false
}

// accept 在匹配时消费当前 token 并返回 true。
func (p *Parser) accept(types ...string) bool {
	if p.at(types...) {
		p.advance()
		return true
	}
	return false
}

// expect 要求当前 token 属于给定集合；无法满足时报错且不移动游标，
// 由调用方决定如何恢复（通常是结束当前语句）。
func (p *Parser) expect(types ...string) bool {
	if p.at(types...) {
		p.advance()
		return true
	}
	p.errorAt(p.cur(), "expected %s but got %s", describeTypes(types), describeToken(p.cur()))
	return false
}

// skipNewlines 跳过换行，用于允许在容器内部、运算符之后换行。
func (p *Parser) skipNewlines() {
	for p.at(tokens.LF) {
		p.advance()
	}
}

// endStatement 消费语句终结符。换行、分号、块结束、文件结束都算合法结束；
// 其他情况报错并跳到下一条语句的起点，保证解析继续进行。
func (p *Parser) endStatement() {
	if p.at(tokens.LF, tokens.Semi) {
		for p.at(tokens.LF, tokens.Semi) {
			p.advance()
		}
		return
	}
	if p.at(tokens.RBRACE, tokens.EOF) {
		return
	}
	p.errorAt(p.cur(), "unexpected %s after statement (missing operator or newline?)",
		describeToken(p.cur()))
	p.synchronize()
}

// synchronize 跳到下一条语句的起点：换行之后、块结束或文件结束。
func (p *Parser) synchronize() {
	for !p.at(tokens.LF, tokens.RBRACE, tokens.EOF) {
		p.advance()
	}
	for p.at(tokens.LF) {
		p.advance()
	}
}

// synchronizeTo 跳到指定结束符（不消费）或换行处。
func (p *Parser) synchronizeTo(end string) {
	for !p.at(end, tokens.LF, tokens.EOF) {
		p.advance()
	}
	if p.at(tokens.LF) {
		p.advance()
	}
}

// forceProgress 保证循环有进展：解析器对任意输入都必须终止。
// （曾经因为缺少这个护栏，畸形输入会把内存吃光。）
func (p *Parser) forceProgress() {
	if p.at(tokens.EOF) {
		return
	}
	p.advance()
}

// ---------------------------------------------------------------------------
// 错误报告
// ---------------------------------------------------------------------------

// 下面三个方法遮蔽了嵌入的 errors.Errors 同名方法，
// 让解析器的所有报错都受 maxErrors 约束。
func (p *Parser) NewErrorF(format string, args ...interface{}) {
	if p.HasError() && len(p.Errs()) >= maxErrors {
		return
	}
	p.Errors.NewErrorF(format, args...)
}

func (p *Parser) NewError(msg string) {
	if p.HasError() && len(p.Errs()) >= maxErrors {
		return
	}
	p.Errors.NewError(msg)
}

func (p *Parser) Push(err error) {
	if p.HasError() && len(p.Errs()) >= maxErrors {
		return
	}
	p.Errors.Push(err)
}

// errorAt 报告一条带位置信息的错误。
func (p *Parser) errorAt(tok *tokens.Token, format string, args ...interface{}) {
	loc := tokens.Locate{Line: 1, Column: 1}
	if tok != nil {
		loc = tok.Loc
	}
	p.NewErrorF("line %d, col %d: %s", loc.Line, loc.Column, fmt.Sprintf(format, args...))
}

func describeToken(tok *tokens.Token) string {
	if tok == nil {
		return "<nil>"
	}
	if tok.IsEOF() {
		return "end of file"
	}
	if tok.Literal == "" || tok.Literal == tok.Type {
		return tok.Type
	}
	return strconv.Quote(tok.Literal)
}

func describeTypes(types []string) string {
	if len(types) == 1 {
		return types[0]
	}
	out := ""
	for i, t := range types {
		if i > 0 {
			out += " or "
		}
		out += t
	}
	return out
}

// ---------------------------------------------------------------------------
// 作用域
// ---------------------------------------------------------------------------

// enterScope 新建一个块作用域并编号（命名刻意区别于函数表，避免冲突）。
func (p *Parser) enterScope() *SymTable {
	p.scopeSeq++
	table := NewInnerSymTable("#blk"+strconv.Itoa(p.scopeSeq), p.SymTable)
	table.ID = p.scopeSeq
	p.SymTable = table
	return table
}

func (p *Parser) exitScope(outer *SymTable) { p.SymTable = outer }

// ---------------------------------------------------------------------------
// 程序与语句
// ---------------------------------------------------------------------------

func (p *Parser) Parse() ast.Statement { return p.parseProgram() }

func (p *Parser) parseProgram() ast.Program {
	var statements []ast.Statement
	p.skipNewlines()
	for !p.at(tokens.EOF) {
		before := p.tokenSeq
		if stmt := p.parseStatement(); stmt != nil {
			statements = append(statements, stmt)
		}
		p.endStatement()
		if p.tokenSeq == before {
			p.forceProgress()
		}
	}
	return ast.Program{Statements: statements}
}

// parseStatement 解析一条语句，返回时 cur 停在该语句的最后一个 token 上。
func (p *Parser) parseStatement() ast.Statement {
	switch p.cur().Type {
	case tokens.Var:
		return p.parseVarStatement()
	case tokens.Return:
		return p.parseReturnStatement()
	case tokens.Break:
		return p.parseBreakStmt()
	case tokens.Func:
		return ast.FuncStatement{Expression: p.parseFuncDef()}
	case tokens.LF, tokens.Semi, tokens.EOF:
		return nil
	}

	expr := p.parseExpr(LOWEST)
	if expr == nil {
		return nil
	}
	switch {
	case p.at(tokens.Assign):
		return p.parseAssignment(expr)
	case p.at(tokens.IPlus, tokens.IMinus, tokens.IMul, tokens.IDiv, tokens.IMod, tokens.IPow, tokens.IFloorDiv):
		return p.parseCompoundAssign(expr)
	}
	return ast.ExprStatement{Expression: expr}
}

// parseAssignment 在已解析出左值之后，继续解析 `= expr`。
// 左值可以是标识符或下标表达式，其余形式报错。
func (p *Parser) parseAssignment(target ast.Expression) ast.Statement {
	assign := *p.cur()
	p.advance()
	p.skipNewlines()
	value := p.parseExpr(LOWEST)

	switch target := target.(type) {
	case ast.IdentNode:
		return ast.AssignStatement{
			Ident:      assign,
			Identifier: target,
			Statement:  value,
		}
	case ast.IndexExpression:
		if _, ok := target.Left.(ast.IdentNode); !ok {
			p.errorAt(&assign, "cannot assign to an element of a non-variable expression")
			return nil
		}
		return ast.ExpressionAssign{
			Token: assign,
			Old:   target.Left,
			Key:   target.Index,
			New:   value,
		}
	default:
		p.errorAt(&assign, "cannot assign to this expression")
		return nil
	}
}

// parseCompoundAssign 处理 x += e 这类原地运算符，编译期翻译成 x = x + e。
func (p *Parser) parseCompoundAssign(target ast.Expression) ast.Statement {
	op := *p.cur()
	p.advance()
	p.skipNewlines()
	value := p.parseExpr(LOWEST)
	return ast.ExprStatement{Expression: ast.InfixExpr{
		Left:  target,
		Right: value,
		Op:    op,
	}}
}

func (p *Parser) parseVarStatement() ast.Statement {
	token := *p.cur() // var
	p.advance()       // var

	if !p.at(tokens.Ident) {
		p.errorAt(p.cur(), "expected a variable name after var but got %s", describeToken(p.cur()))
		p.synchronize()
		return nil
	}
	name := p.cur()
	ident := ast.IdentNode{Token: *name, Value: name.Literal}
	if p.SymTable.Defined(name.Literal) {
		p.errorAt(name, "variable %s is already declared in this scope", strconv.Quote(name.Literal))
	}
	p.SymTable.Define(name.Literal, I)
	p.advance() // 标识符

	if !p.expect(tokens.Assign) {
		p.synchronize()
		return &ast.VarStatement{Token: token, Indent: ident}
	}
	p.skipNewlines()
	value := p.parseExpr(LOWEST)

	// 方法调用作为初始化表达式时要用 VarMethodCall：
	// 编译器要保留方法调用的返回值（MethodCall 自身会弹掉它）
	if _, ok := value.(ast.MethodCall); ok {
		return ast.VarMethodCall{Token: token, Indent: ident, Value: value}
	}
	return ast.VarStatement{Token: token, Indent: ident, Value: value}
}

func (p *Parser) parseReturnStatement() ast.Statement {
	token := *p.cur() // return
	p.advance()

	if p.at(tokens.LF, tokens.Semi, tokens.RBRACE, tokens.EOF) {
		return ast.ReturnStatement{Token: token}
	}
	value := p.parseExpr(LOWEST)
	return ast.ReturnStatement{Token: token, ReturnVal: value}
}

func (p *Parser) parseBreakStmt() ast.Statement {
	token := *p.cur() // break
	p.advance()
	return ast.ExprStatement{Expression: ast.BreakExpr{Token: token}}
}

// parseBlock 解析 `{ statements }`，并建立块作用域。
func (p *Parser) parseBlock() ast.BlockStatement {
	token := *p.cur() // {
	p.expect(tokens.LBRACE)
	p.skipNewlines()

	outer := p.SymTable
	block := p.enterScope()
	var statements []ast.Statement
	for !p.at(tokens.RBRACE, tokens.EOF) {
		before := p.tokenSeq
		if stmt := p.parseStatement(); stmt != nil {
			statements = append(statements, stmt)
		}
		p.endStatement()
		if p.tokenSeq == before {
			p.forceProgress()
		}
	}
	p.expect(tokens.RBRACE)
	p.exitScope(outer)

	return ast.BlockStatement{
		Token:      token,
		Statements: statements,
		ScopeID:    block.ID,
	}
}

// requireBlock 解析语句体，要求必须用 {} 包裹。
func (p *Parser) requireBlock(what string) ast.BlockStatement {
	p.skipNewlines()
	if !p.at(tokens.LBRACE) {
		p.errorAt(p.cur(), `%s body must be wrapped in "{}" but got %s`, what, describeToken(p.cur()))
		return ast.BlockStatement{Token: *p.cur()}
	}
	return p.parseBlock()
}

// ---------------------------------------------------------------------------
// if / for / def
// ---------------------------------------------------------------------------

func (p *Parser) parseIfExpression() ast.Expression {
	token := *p.cur() // if
	p.advance()

	p.expect(tokens.LParen)
	cond := p.parseExpr(LOWEST)
	p.expect(tokens.RParen)

	consequence := p.requireBlock("if")
	result := ast.IfExpression{
		Token:       token,
		Condition:   cond,
		Consequence: &consequence,
	}
	// else 允许换行写在 } 之后；用前瞻判断，
	// 不会因为"想找 else"而吞掉语句分隔符
	if p.elseAhead() {
		p.skipNewlines()
		p.expect(tokens.Else)
		alternative := p.requireBlock("else")
		result.Alternative = &alternative
	}
	return result
}

// elseAhead 判断后面（可跨换行）是否紧跟 else。
// 注意要先看当前 token：`}else` 这种同行写法 cur 就是 else。
func (p *Parser) elseAhead() bool {
	if p.at(tokens.Else) {
		return true
	}
	for k := 1; ; k++ {
		tok := p.token(k)
		if tok.IsLF() {
			continue
		}
		return tok.Type == tokens.Else
	}
}

func (p *Parser) parseForExpr() ast.Expression {
	token := *p.cur() // for
	p.advance()

	p.expect(tokens.LParen)
	outer := p.SymTable
	loopScope := p.enterScope() // for 头与循环体共用一个作用域

	var init, each ast.Statement
	var cond ast.Expression
	p.skipNewlines()
	if !p.at(tokens.RParen) {
		// 形式一：for(init; cond; each)
		// 形式二：for(cond)
		if !p.at(tokens.Semi) {
			init = p.parseStatement()
			if p.at(tokens.RParen) {
				// while 形式：唯一的一段是条件
				if es, ok := init.(ast.ExprStatement); ok {
					cond, init = es.Expression, nil
				}
			}
		}
		if p.accept(tokens.Semi) {
			if !p.at(tokens.Semi, tokens.RParen) {
				cond = p.parseExpr(LOWEST)
			}
		}
		if p.accept(tokens.Semi) {
			if !p.at(tokens.RParen) {
				each = p.parseStatement()
			}
		}
	}
	p.expect(tokens.RParen)

	body := p.requireBlock("for")
	p.exitScope(outer)

	return ast.ForExpression{
		Token:       token,
		Condition:   cond,
		InitCond:    init,
		EachOperate: each,
		Loop:        &body,
		ScopeID:     loopScope.ID,
	}
}

func (p *Parser) parseFuncDef() ast.Expression {
	token := *p.cur() // def
	p.advance()

	if !p.at(tokens.Ident) {
		p.errorAt(p.cur(), "expected a function name after def but got %s", describeToken(p.cur()))
		p.synchronize()
		return nil
	}
	name := p.cur().Literal
	p.advance()

	// 每个函数声明拥有独立的作用域编号，同名函数互不覆盖
	p.scopeSeq++
	funcScope := p.scopeSeq
	outer := p.SymTable
	outer.DefineFunc(name, funcScope)
	p.SymTable = NewFuncSymTable(name, outer, funcScope)

	params := p.parseParamList()
	for _, param := range params {
		p.SymTable.Define(param.Value, I)
	}
	body := p.requireBlock("function")
	p.SymTable = outer

	return ast.FuncDef{
		Token:      token,
		Parameters: params,
		FuncBody:   &body,
		Name:       name,
		ScopeID:    funcScope,
	}
}

func (p *Parser) parseParamList() []ast.IdentNode {
	var params []ast.IdentNode
	if !p.expect(tokens.LParen) {
		return params
	}
	p.skipNewlines()
	for !p.at(tokens.RParen, tokens.EOF) {
		if !p.at(tokens.Ident) {
			p.errorAt(p.cur(), "expected a parameter name but got %s", describeToken(p.cur()))
			break
		}
		params = append(params, ast.IdentNode{Token: *p.cur(), Value: p.cur().Literal})
		p.advance()
		p.skipNewlines()
		if !p.accept(tokens.Comma) {
			break
		}
		p.skipNewlines()
	}
	p.expect(tokens.RParen)
	return params
}

// ---------------------------------------------------------------------------
// 表达式（Pratt）
// ---------------------------------------------------------------------------

// parseExpr 是优先级爬升入口：先解析前缀，再按优先级吸收中缀。
// 游标模型：cur 始终是"下一个待消费的 token"，因此这里判断的是 cur。
func (p *Parser) parseExpr(precedence int) ast.Expression {
	prefix, ok := p.prefixFns[p.cur().Type]
	if !ok {
		// 包括 EOF：`1+` 这种缺少操作数的情况必须报错，不能静默返回 nil
		p.errorAt(p.cur(), "unexpected %s in expression", describeToken(p.cur()))
		return nil
	}
	left := prefix()
	for left != nil && precedence < p.curPrecedence() {
		infix, ok := p.infixFns[p.cur().Type]
		if !ok {
			break
		}
		left = infix(left)
	}
	return left
}

func (p *Parser) curPrecedence() int { return precedenceOf(p.cur().Type) }

func precedenceOf(typ string) int {
	if prec, ok := precedences[typ]; ok {
		return prec
	}
	return LOWEST
}

func (p *Parser) parseInfixExpr(left ast.Expression) ast.Expression {
	op := *p.cur()
	precedence := p.curPrecedence()
	p.advance()
	p.skipNewlines() // 允许运算符后换行
	right := p.parseExpr(precedence)
	return ast.InfixExpr{Left: left, Right: right, Op: op}
}

func (p *Parser) parsePrefixExpr() ast.Expression {
	op := *p.cur()
	p.advance()
	priority := PREFIX
	if op.Type == tokens.Not {
		// not 的右操作数吞下整个比较：not a == b 等价于 not (a == b)
		priority = Not
	}
	return ast.PrefixExpr{Op: op, Right: p.parseExpr(priority)}
}

func (p *Parser) parseGroupedExpr() ast.Expression {
	p.expect(tokens.LParen)
	inner := p.parseExpr(LOWEST)
	p.expect(tokens.RParen)
	return inner
}

func (p *Parser) parseInt() ast.Expression {
	token := *p.cur()
	value, err := strconv.ParseInt(token.Literal, 10, 64)
	p.advance()
	if err != nil {
		p.errorAt(&token, "invalid integer literal %s", token.Literal)
		return nil
	}
	return ast.IntNode{Token: token, Value: int(value)}
}

func (p *Parser) parseFloat() ast.Expression {
	token := *p.cur()
	value, err := strconv.ParseFloat(token.Literal, 64)
	p.advance()
	if err != nil {
		p.errorAt(&token, "invalid float literal %s", token.Literal)
		return nil
	}
	return ast.FloatNode{Token: token, Value: value}
}

func (p *Parser) parseIdentifier() ast.Expression {
	token := *p.cur()
	p.advance()
	return ast.IdentNode{Token: token, Value: token.Literal}
}

func (p *Parser) parseString() ast.Expression {
	token := *p.cur()
	p.advance()
	return ast.StringNode{Token: token, Value: token.Literal}
}

func (p *Parser) parseBoolean() ast.Expression {
	token := *p.cur()
	value, err := strconv.ParseBool(token.Literal)
	p.advance()
	if err != nil {
		p.errorAt(&token, "invalid boolean literal %s", token.Literal)
		return nil
	}
	return ast.BooleanNode{Token: token, Value: value}
}

func (p *Parser) parseNone() ast.Expression {
	token := *p.cur()
	p.advance()
	return ast.NoneNode{Token: token}
}

func (p *Parser) parseArray() ast.Expression {
	token := *p.cur() // [
	elements := p.parseExprList(tokens.LBRACKET, tokens.RBRACKET, "array literal")
	return ast.Array{Token: token, Elements: elements}
}

// parseCallExpr 解析函数调用实参表（`(` 已作为中缀 token 命中）。
func (p *Parser) parseCallExpr(function ast.Expression) ast.Expression {
	token := *p.cur() // (
	args := p.parseExprList(tokens.LParen, tokens.RParen, "call arguments")
	return ast.FuncCallExpr{Token: token, Function: function, Arguments: args}
}

// parseExprList 解析以 open 开始、close 结束的表达式列表，允许内部换行。
// 返回时 cur 位于 close 之后。
func (p *Parser) parseExprList(open, close, what string) []ast.Expression {
	var list []ast.Expression
	if !p.expect(open) {
		return list
	}
	p.skipNewlines()
	for !p.at(close, tokens.EOF) {
		before := p.tokenSeq
		if elem := p.parseExpr(LOWEST); elem != nil {
			list = append(list, elem)
		}
		p.skipNewlines()
		if p.accept(tokens.Comma) {
			p.skipNewlines()
		} else if !p.at(close) {
			p.errorAt(p.cur(), "%s expects ',' or %s but got %s",
				what, close, describeToken(p.cur()))
			p.synchronizeTo(close)
			break
		}
		if p.tokenSeq == before {
			p.forceProgress()
		}
	}
	p.expect(close)
	return list
}

// parseMap 解析 `{ key: value, ... }`。
func (p *Parser) parseMap() ast.Expression {
	token := *p.cur() // {
	p.expect(tokens.LBRACE)
	p.skipNewlines()

	var keys, items []ast.Expression
	for !p.at(tokens.RBRACE, tokens.EOF) {
		before := p.tokenSeq
		key := p.parseExpr(LOWEST)
		if !p.at(tokens.Colon) {
			p.errorAt(p.cur(), `map entry needs ":" but got %s`, describeToken(p.cur()))
			p.synchronizeTo(tokens.RBRACE)
			break
		}
		p.advance() // ':'
		p.skipNewlines()
		value := p.parseExpr(LOWEST)
		if key == nil || value == nil {
			p.errorAt(p.cur(), "incomplete map entry near %s", describeToken(p.cur()))
			break
		}
		keys = append(keys, key)
		items = append(items, value)

		p.skipNewlines()
		if p.accept(tokens.Comma) {
			p.skipNewlines()
		} else if !p.at(tokens.RBRACE) {
			p.errorAt(p.cur(), `map entry expects "," or "}" but got %s`, describeToken(p.cur()))
			p.synchronizeTo(tokens.RBRACE)
			break
		}
		if p.tokenSeq == before {
			p.forceProgress()
		}
	}
	p.expect(tokens.RBRACE)
	return ast.Map{Token: token, Keys: keys, Items: items}
}

// parseIndexExpr 解析下标或切片：a[i] / a[b:e] / a[b:e:s]。
func (p *Parser) parseIndexExpr(left ast.Expression) ast.Expression {
	token := *p.cur() // [
	p.advance()

	p.skipNewlines()
	var start, end, step ast.Expression
	if !p.at(tokens.Colon, tokens.RBRACKET, tokens.EOF) {
		start = p.parseExpr(LOWEST)
	}
	if !p.at(tokens.Colon) {
		p.expect(tokens.RBRACKET)
		return ast.IndexExpression{Token: token, Left: left, Index: start}
	}

	p.advance() // 第一个 ':'
	p.skipNewlines()
	if !p.at(tokens.Colon, tokens.RBRACKET, tokens.EOF) {
		end = p.parseExpr(LOWEST)
	}
	if p.at(tokens.Colon) {
		p.advance()
		p.skipNewlines()
		if !p.at(tokens.RBRACKET, tokens.EOF) {
			step = p.parseExpr(LOWEST)
		}
	}
	p.expect(tokens.RBRACKET)

	return ast.IndexExpression{
		Token: token,
		Left:  left,
		Index: ast.IndexSlice{Start: start, End: end, Step: step},
	}
}

// parseMethodCall 解析方法链：a.b(1).c(2)。
func (p *Parser) parseMethodCall(left ast.Expression) ast.Expression {
	token := *p.cur() // .
	call := ast.MethodCall{Token: token, Left: left}

	for p.at(tokens.Dot) {
		p.advance() // '.'
		if !p.at(tokens.Ident) {
			p.errorAt(p.cur(), "expected a method name after '.' but got %s", describeToken(p.cur()))
			break
		}
		name := p.cur()
		p.SymTable.Methods.Add(name.Literal)
		call.Methods = append(call.Methods, ast.MethodNode{Token: *name, Value: name.Literal})
		p.advance() // 方法名

		var args []ast.Expression
		if p.at(tokens.LParen) {
			args = p.parseExprList(tokens.LParen, tokens.RParen, "method arguments")
		}
		call.Arguments = append(call.Arguments, args)
	}
	return call
}
