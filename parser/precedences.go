package parser

import (
	"Interpreter/ast"
	"Interpreter/tokens"
)

const (
	_ int = iota
	LOWEST
	Method    // .
	Eq        // == !=
	GreatLess // < > <= >=
	SUM       // +,-
	PRODUCT   // *,/,//
	POW       //**
	PREFIX    // -x,!x
	COMPARE   // and,or,not
	CALL      //()
	Index     //[]
	Highest
)

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)

var precedences = map[string]int{
	tokens.Plus:     SUM,
	tokens.Minus:    SUM,
	tokens.Mul:      PRODUCT,
	tokens.Div:      PRODUCT,
	tokens.Mod:      POW,
	tokens.Pow:      POW,
	tokens.LParen:   CALL,
	tokens.Dot:      Method,
	tokens.Equal:    Eq,
	tokens.LT:       GreatLess,
	tokens.GT:       GreatLess,
	tokens.LTEq:     GreatLess,
	tokens.GTEq:     GreatLess,
	tokens.And:      COMPARE,
	tokens.Or:       COMPARE,
	tokens.Not:      COMPARE,
	tokens.LBRACKET: Index,
}
