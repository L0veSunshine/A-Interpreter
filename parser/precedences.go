package parser

import (
	"Interpreter/ast"
	"Interpreter/tokens"
)

// 优先级从低到高，与 Python 保持一致：
// or < and < not < 比较 < 加减 < 乘除模 < 幂 < 一元 < 属性/调用/下标
const (
	LOWEST    int = iota
	Or            // or
	And           // and
	Not           // not / !
	Eq            // == !=
	GreatLess     // < > <= >=
	SUM           // +,-
	PRODUCT       // *,/,%
	POW           //**
	PREFIX        // -x
	Method        // .
	CALL          //()
	Index         //[]
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
	tokens.Mod:      PRODUCT,
	tokens.FloorDiv: PRODUCT,
	tokens.Pow:      POW,
	tokens.LParen:   CALL,
	tokens.Dot:      Method,
	tokens.Equal:    Eq,
	tokens.NotEq:    Eq,
	tokens.LT:       GreatLess,
	tokens.GT:       GreatLess,
	tokens.LTEq:     GreatLess,
	tokens.GTEq:     GreatLess,
	tokens.And:      And,
	tokens.Or:       Or,
	tokens.Not:      Not,
	tokens.LBRACKET: Index,
}
