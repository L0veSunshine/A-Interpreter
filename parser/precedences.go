package parser

import (
	"Interpreter/ast"
	"Interpreter/tokens"
)

const (
	_ int = iota
	LOWEST
	Eq        // == !=
	GreatLess // < > <= >=
	SUM       // +,-
	PRODUCT   // *,/,//
	POW       //**
	PREFIX    // -x,!x
	COMPARE   // and,or,not
	CALL
)

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)

var precedences = map[string]int{
	tokens.Plus:   SUM,
	tokens.Minus:  SUM,
	tokens.Mul:    PRODUCT,
	tokens.Div:    PRODUCT,
	tokens.Mod:    POW,
	tokens.Pow:    POW,
	tokens.LParen: CALL,
	tokens.Equal:  Eq,
	tokens.LT:     GreatLess,
	tokens.GT:     GreatLess,
	tokens.LTEq:   GreatLess,
	tokens.GTEq:   GreatLess,
	tokens.And:    COMPARE,
	tokens.Or:     COMPARE,
	tokens.Not:    COMPARE,
}
