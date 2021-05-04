package ast

import (
	"Interpreter/tokens"
	"fmt"
)

type InfixExpr struct {
	Left, Right Node
	Op          tokens.Token
}

func (ie InfixExpr) expressionNode() {}
func (ie InfixExpr) TokenLiteral() string {
	return ie.Op.Literal
}
func (ie InfixExpr) Str() string {
	return fmt.Sprintf("[%s %s %s]",
		ie.Left.Str(),
		ie.Op.Literal,
		ie.Right.Str())
}

type PrefixExpr struct {
	Op    tokens.Token
	Right Expression
}

func (pe PrefixExpr) expressionNode() {}
func (pe PrefixExpr) TokenLiteral() string {
	return pe.Op.Literal
}

func (pe PrefixExpr) Str() string {
	return fmt.Sprintf("[%s %s]",
		pe.Op.Literal,
		pe.Right.Str())
}

type NumberNode struct {
	Token tokens.Token
	Value float64
}

func (n NumberNode) expressionNode() {}
func (n NumberNode) TokenLiteral() string {
	return n.Token.Literal
}

func (n NumberNode) Str() string {
	return n.Token.Literal
}

type IdentNode struct {
	Token tokens.Token
	Value string
}

func (i IdentNode) expressionNode() {}
func (i IdentNode) TokenLiteral() string {
	return i.Token.Literal
}

func (i IdentNode) Str() string {
	return i.Value
}

type BooleanNode struct {
	Token tokens.Token
	Value string
}

func (b BooleanNode) expressionNode() {}
func (b BooleanNode) TokenLiteral() string {
	return b.Token.Literal
}

func (b BooleanNode) Str() string {
	return b.Value
}

type StringNode struct {
	Token tokens.Token
	Value string
}

func (s StringNode) expressionNode() {}
func (s StringNode) TokenLiteral() string {
	return s.Token.Literal
}

func (s StringNode) Str() string {
	return s.Value
}
