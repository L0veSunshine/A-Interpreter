package main

import (
	"fmt"
)

type Node interface {
	TokenLiteral() string
	String() string
}

type Expression interface {
	Node
}

type InfixExpr struct {
	Left, Right Node
	Op          Token
}

func (ie InfixExpr) TokenLiteral() string {
	return ie.Op.Literal
}

func (ie InfixExpr) String() string {
	return fmt.Sprintf("[%s %s %s]",
		ie.Left.String(),
		ie.Op.Literal,
		ie.Right.String())
}

type PrefixExpr struct {
	Op    Token
	Right Expression
}

func (pe PrefixExpr) TokenLiteral() string {
	return pe.Op.Literal
}

func (pe PrefixExpr) String() string {
	return fmt.Sprintf("[%s %s]",
		pe.Op.Literal,
		pe.Right.String())
}

type NumberNode struct {
	Token Token
	Value float64
}

func (n NumberNode) TokenLiteral() string {
	return n.Token.Literal
}

func (n NumberNode) String() string {
	return n.Token.Literal
}
