package main

import (
	"strconv"
)

type Parser struct {
	lex *Lexer
	curToken,
	peekToken *Token
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
	p.lex.loc = &Locate{Column: 1, Line: 1}

	p.regPrefixFn(LParen, p.expr)
	p.regPrefixFn(Plus, p.parsePrefixExpr)
	p.regPrefixFn(Minus, p.parsePrefixExpr)
	p.regPrefixFn(Number, p.parseNumber)

	p.regInfixFn(Minus, p.parseInfixExpr)
	p.regInfixFn(Plus, p.parseInfixExpr)
	p.regInfixFn(Mul, p.parseInfixExpr)
	p.regInfixFn(Div, p.parseInfixExpr)
	p.regInfixFn(Floor, p.parseInfixExpr)
	p.regInfixFn(Pow, p.parseInfixExpr)

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
func (p *Parser) eatPeek(Type string) {
	if p.peekToken.Type == Type {
		p.next()
		return
	}
	p.NewErrorF("Want %s but get %s.(col%d,line%d)", Type, p.curToken.Type,
		p.curToken.Loc.Column, p.curToken.Loc.Line)
}

func (p *Parser) parseExpr(precedence int) Expression {
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

func (p *Parser) parsePrefixExpr() Expression {
	token := *p.curToken
	p.next()
	right := p.parseExpr(PREFIX)
	return PrefixExpr{
		Op:    token,
		Right: right,
	}
}

func (p *Parser) parseInfixExpr(left Expression) Expression {
	op := *p.curToken
	precedence := p.curPrecedence()
	p.next()
	right := p.parseExpr(precedence)
	return InfixExpr{
		Left:  left,
		Right: right,
		Op:    op,
	}
}

func (p *Parser) parseNumber() Expression {
	var token = *p.curToken
	floatVal, e := strconv.ParseFloat(p.curToken.Literal, 64)
	if e != nil {
		p.Push(e)
		return nil
	}
	return NumberNode{
		Token: token,
		Value: floatVal,
	}
}

//func (p *Parser) factor() Node {
//	token := p.curToken
//	switch token.Type {
//	case Plus:
//		p.eat(Plus)
//		return PrefixExpr{
//			Right: p.factor(),
//			Op:    *token,
//		}
//	case Minus:
//		p.eat(Minus)
//		return PrefixExpr{
//			Right: p.factor(),
//			Op:    *token,
//		}
//	case LParen:
//		p.eat(LParen)
//		val := p.expr()
//		p.eat(RParen)
//		return val
//	case Number:
//		floatVal, e := strconv.ParseFloat(p.curToken.Literal, 64)
//		var token = *p.curToken
//		if e != nil {
//			p.Push(e)
//			return nil
//		}
//		p.eat(Number)
//		return NumberNode{
//			Token: token,
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
//		token := *p.curToken
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
//			Op:    token,
//		}
//	}
//	return node
//}

//func (p *Parser) midFactor() Node {
//	var node Node
//	node = p.factor()
//	for p.curToken.Type == Pow {
//		token := *p.curToken
//		if p.curToken.Type == Pow {
//			p.eat(Pow)
//		}
//		node = InfixExpr{
//			Left:  node,
//			Right: p.factor(),
//			Op:    token,
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

func (p *Parser) expr() Expression {
	p.eat(LParen)
	exp := p.parseExpr(LOWEST)
	p.eatPeek(RParen)
	return exp
}

func (p *Parser) Parse() (node Expression) {
	node = p.parseExpr(LOWEST)
	p.next()
	cur := p.curToken
	if cur.Type != EOF {
		p.NewErrorF("Unexpect token %s, at loc%d, line%d.",
			cur.Quote(), cur.Loc.Column, cur.Loc.Line)
	}
	return node
}
