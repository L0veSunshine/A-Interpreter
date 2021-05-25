package ast

import (
	"Interpreter/tokens"
	"fmt"
	"strconv"
	"strings"
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

type IntNode struct {
	Token tokens.Token
	Value int
}

func (n IntNode) expressionNode() {}
func (n IntNode) TokenLiteral() string {
	return n.Token.Literal
}

func (n IntNode) Str() string {
	return n.Token.Literal
}

type FloatNode struct {
	Token tokens.Token
	Value float64
}

func (fn FloatNode) expressionNode() {}
func (fn FloatNode) TokenLiteral() string {
	return fn.Token.Literal
}

func (fn FloatNode) Str() string {
	return fn.Token.Literal
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
	Value bool
}

func (b BooleanNode) expressionNode() {}
func (b BooleanNode) TokenLiteral() string {
	return b.Token.Literal
}

func (b BooleanNode) Str() string {
	return strconv.FormatBool(b.Value)
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

type IfExpression struct {
	Token       tokens.Token //"if" token
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement
}

func (i IfExpression) expressionNode() {}
func (i IfExpression) TokenLiteral() string {
	return i.Token.Literal
}

func (i IfExpression) Str() string {
	var sb strings.Builder
	sb.WriteString("If (")
	sb.WriteString(i.Condition.Str() + ") ")
	sb.WriteString(i.Consequence.Str())
	if i.Alternative != nil {
		sb.WriteString("Else (")
		sb.WriteString(i.Alternative.Str() + ") ")
	}
	return sb.String()
}

type ForExpression struct {
	Token     tokens.Token
	Condition Expression
	Loop      *BlockStatement
}

func (fe ForExpression) expressionNode() {}
func (fe ForExpression) TokenLiteral() string {
	return fe.Token.Literal
}

func (fe ForExpression) Str() string {
	var sb strings.Builder
	sb.WriteString("For (" + fe.Condition.Str() + ") ")
	sb.WriteString(fe.Loop.Str())
	return sb.String()
}
