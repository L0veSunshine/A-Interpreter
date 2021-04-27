package main

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
	prefixParseFn func() Expression
	infixParseFn  func(Expression) Expression
)

var precedences = map[string]int{
	Plus:   SUM,
	Minus:  SUM,
	Mul:    PRODUCT,
	Div:    PRODUCT,
	Floor:  PRODUCT,
	Pow:    POW,
	LParen: CALL,
}
