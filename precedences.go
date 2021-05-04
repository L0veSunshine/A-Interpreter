package main

import (
	"Interpreter/ast"
	"Interpreter/tokens"
)

const (
	_ int = iota
	LOWEST
	SUM     // +,-
	PRODUCT // *,/,//
	POW
	PREFIX // -x,!x
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
	tokens.Floor:  PRODUCT,
	tokens.Pow:    POW,
	tokens.LParen: CALL,
}
