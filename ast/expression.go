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

type MethodNode struct {
	Token tokens.Token
	Value string
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
	return "'" + s.Value + "'"
}

type NoneNode struct {
	Token tokens.Token
}

func (n NoneNode) expressionNode() {}
func (n NoneNode) TokenLiteral() string {
	return n.Token.Literal
}

func (n NoneNode) Str() string {
	return "None"
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
		sb.WriteString(" Else (")
		sb.WriteString(i.Alternative.Str() + ") ")
	}
	return sb.String()
}

type ForExpression struct {
	Token       tokens.Token
	InitCond    Statement
	Condition   Expression
	EachOperate Statement
	Loop        *BlockStatement
}

func (fe ForExpression) expressionNode() {}
func (fe ForExpression) TokenLiteral() string {
	return fe.Token.Literal
}

func (fe ForExpression) Str() string {
	var sb strings.Builder
	sb.WriteString("For (")
	if fe.InitCond != nil {
		sb.WriteString(fe.InitCond.Str() + ";")
	} else {
		sb.WriteString(";")
	}
	if fe.Condition != nil {
		sb.WriteString(fe.Condition.Str() + ";")
	} else {
		sb.WriteString(";")
	}
	if fe.EachOperate != nil {
		sb.WriteString(fe.EachOperate.Str())
	}
	sb.WriteString(")")
	sb.WriteString(fe.Loop.Str())
	return sb.String()
}

type FuncDef struct {
	Token      tokens.Token
	Parameters []IdentNode
	FuncBody   *BlockStatement
	Name       string
}

func (fd FuncDef) expressionNode() {}
func (fd FuncDef) TokenLiteral() string {
	return fd.Token.Literal
}

func (fd FuncDef) Str() string {
	var sb strings.Builder
	var params []string
	for _, p := range fd.Parameters {
		params = append(params, p.Str())
	}
	sb.WriteString(fmt.Sprintf("Func: %s(", fd.Name))
	sb.WriteString(strings.Join(params, ",") + ")\n{")
	sb.WriteString(fd.FuncBody.Str() + "}")
	return sb.String()
}

type FuncCallExpr struct {
	Token     tokens.Token
	Function  Expression
	Arguments []Expression
}

func (fc FuncCallExpr) expressionNode() {}
func (fc FuncCallExpr) TokenLiteral() string {
	return fc.Token.Literal
}

func (fc FuncCallExpr) Str() string {
	var sb strings.Builder
	var args []string
	for _, arg := range fc.Arguments {
		args = append(args, arg.Str())
	}
	sb.WriteString(fc.Function.Str())
	sb.WriteString("(" + strings.Join(args, ",") + ")")
	return sb.String()
}

type Array struct {
	Token    tokens.Token
	Elements []Expression
}

func (a Array) expressionNode() {}
func (a Array) TokenLiteral() string {
	return a.Token.Literal
}

func (a Array) Str() string {
	var sb strings.Builder
	sb.WriteString("[")
	if len(a.Elements) > 0 {
		for i := 0; i < len(a.Elements)-1; i++ {
			sb.WriteString(a.Elements[i].Str() + ",")
		}
		sb.WriteString(a.Elements[len(a.Elements)-1].Str())
		sb.WriteString("]")
		return sb.String()
	}
	return "[]"
}

type IndexSlice struct {
	Start, End, Step Expression
}

func (is IndexSlice) expressionNode() {}
func (is IndexSlice) TokenLiteral() string {
	return ""
}

func (is IndexSlice) Str() string {
	var sb strings.Builder
	var start, end, step string
	if is.Start != nil {
		start = is.Start.Str()
	} else {
		start = ""
	}
	if is.End != nil {
		end = is.End.Str()
	} else {
		end = ""
	}
	if is.Step != nil {
		step = is.Step.Str()
	} else {
		step = ""
	}
	sb.WriteString(start + ":" + end + ":" + step)
	return sb.String()
}

type IndexExpression struct {
	Token tokens.Token
	Left,
	Index Expression
}

func (i IndexExpression) expressionNode() {}
func (i IndexExpression) TokenLiteral() string {
	return i.Token.Literal
}

func (i IndexExpression) Str() string {
	var sb strings.Builder
	sb.WriteString(i.Left.Str())
	sb.WriteString("[" + i.Index.Str() + "]")
	return sb.String()
}

type Map struct {
	Token tokens.Token
	Keys,
	Items []Expression
}

func (m Map) expressionNode() {}
func (m Map) TokenLiteral() string {
	return m.Token.Literal
}

func (m Map) Str() string {
	var sb strings.Builder
	sb.WriteString("{")
	for i := 0; i < len(m.Keys); i++ {
		if i == len(m.Keys)-1 {
			sb.WriteString(m.Keys[i].Str() + ":" + m.Items[i].Str())
		} else {
			sb.WriteString(m.Keys[i].Str() + ":" + m.Items[i].Str() + ", ")
		}
	}
	sb.WriteString("}")
	return sb.String()
}

func (mn MethodNode) expressionNode() {}
func (mn MethodNode) TokenLiteral() string {
	return mn.Token.Literal
}

func (mn MethodNode) Str() string {
	return mn.Value
}

type MethodCall struct {
	Token     tokens.Token
	Left      Expression
	Methods   []Expression
	Arguments [][]Expression
}

func (mc MethodCall) expressionNode() {}
func (mc MethodCall) TokenLiteral() string {
	return mc.Token.Literal
}

func (mc MethodCall) Str() string {
	var sb strings.Builder
	sb.WriteString(mc.Left.Str() + ".")
	for i, m := range mc.Methods {
		sb.WriteString(m.Str() + "(")
		for idx, arg := range mc.Arguments[i] {
			if idx != len(mc.Arguments[i])-1 {
				sb.WriteString(arg.Str() + ",")
			} else {
				sb.WriteString(arg.Str())
			}
		}
		sb.WriteString(")")
		if i != len(mc.Methods)-1 {
			sb.WriteString(".")
		}
	}
	return sb.String()
}
