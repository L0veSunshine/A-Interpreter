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

func (ie InfixExpr) Line() int {
	return ie.Op.Loc.Line
}

func (ie InfixExpr) expressionNode() {}
func (ie InfixExpr) TokenLiteral() string {
	return ie.Op.Literal
}
func (ie InfixExpr) Str() string {
	return fmt.Sprintf("[%s %s %s]",
		str(ie.Left),
		ie.Op.Literal,
		str(ie.Right))
}

type PrefixExpr struct {
	Op    tokens.Token
	Right Expression
}

func (pe PrefixExpr) Line() int {
	return pe.Op.Loc.Line
}

func (pe PrefixExpr) expressionNode() {}
func (pe PrefixExpr) TokenLiteral() string {
	return pe.Op.Literal
}

func (pe PrefixExpr) Str() string {
	return fmt.Sprintf("[%s %s]",
		pe.Op.Literal,
		str(pe.Right))
}

type IntNode struct {
	Token tokens.Token
	Value int
}

func (n IntNode) Line() int {
	return n.Token.Loc.Line
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

func (fn FloatNode) Line() int {
	return fn.Token.Loc.Line
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

func (i IdentNode) Line() int {
	return i.Token.Loc.Line
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

func (mn MethodNode) Line() int {
	return mn.Token.Loc.Line
}

type BooleanNode struct {
	Token tokens.Token
	Value bool
}

func (b BooleanNode) Line() int {
	return b.Token.Loc.Line
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

func (s StringNode) Line() int {
	return s.Token.Loc.Line
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

func (n NoneNode) Line() int {
	return n.Token.Loc.Line
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

func (i IfExpression) Line() int {
	return i.Token.Loc.Line
}

func (i IfExpression) expressionNode() {}
func (i IfExpression) TokenLiteral() string {
	return i.Token.Literal
}

func (i IfExpression) Str() string {
	var sb strings.Builder
	sb.WriteString("If (")
	sb.WriteString(str(i.Condition))
	sb.WriteString(") ")
	sb.WriteString(i.Consequence.Str())
	if i.Alternative != nil {
		sb.WriteString(" Else (")
		sb.WriteString(i.Alternative.Str())
		sb.WriteString(") ")
	}
	return sb.String()
}

type ForExpression struct {
	Token       tokens.Token
	InitCond    Statement
	Condition   Expression
	EachOperate Statement
	Loop        *BlockStatement
	// ScopeID 是 for 头部引入的作用域，init 里的 var 只活到这个循环结束
	ScopeID int
}

func (fe ForExpression) Line() int {
	return fe.Token.Loc.Line
}

func (fe ForExpression) expressionNode() {}
func (fe ForExpression) TokenLiteral() string {
	return fe.Token.Literal
}

func (fe ForExpression) Str() string {
	var sb strings.Builder
	sb.WriteString("For (")
	if fe.InitCond != nil {
		sb.WriteString(fe.InitCond.Str())
		sb.WriteByte(';')
	} else {
		sb.WriteString(";")
	}
	if fe.Condition != nil {
		sb.WriteString(str(fe.Condition))
		sb.WriteByte(';')
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
	// ScopeID 是该函数自己的作用域编号，用于解析同名函数与局部槽位
	ScopeID int
}

func (fd FuncDef) Line() int {
	return fd.Token.Loc.Line
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
	sb.WriteString(strings.Join(params, ","))
	sb.WriteString(")\n{")
	sb.WriteString(str(fd.FuncBody))
	sb.WriteByte('}')
	return sb.String()
}

type FuncCallExpr struct {
	Token     tokens.Token
	Function  Expression
	Arguments []Expression
}

func (fc FuncCallExpr) Line() int {
	return fc.Token.Loc.Line
}

func (fc FuncCallExpr) expressionNode() {}
func (fc FuncCallExpr) TokenLiteral() string {
	return fc.Token.Literal
}

func (fc FuncCallExpr) Str() string {
	var sb strings.Builder
	var args []string
	for _, arg := range fc.Arguments {
		args = append(args, str(arg))
	}
	sb.WriteString(str(fc.Function))
	sb.WriteByte('(')
	sb.WriteString(strings.Join(args, ","))
	sb.WriteByte(')')
	return sb.String()
}

type Array struct {
	Token    tokens.Token
	Elements []Expression
}

func (a Array) Line() int {
	return a.Token.Loc.Line
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
			sb.WriteString(str(a.Elements[i]))
			sb.WriteByte(',')
		}
		sb.WriteString(str(a.Elements[len(a.Elements)-1]))
		sb.WriteString("]")
		return sb.String()
	}
	return "[]"
}

type IndexSlice struct {
	Start, End, Step Expression
}

func (is IndexSlice) Line() int {
	// 切片没有自己的 token，取第一个有位置的子表达式
	if is.Start != nil {
		return is.Start.Line()
	}
	if is.End != nil {
		return is.End.Line()
	}
	if is.Step != nil {
		return is.Step.Line()
	}
	return 0
}

func (is IndexSlice) expressionNode() {}
func (is IndexSlice) TokenLiteral() string {
	return ""
}

func (is IndexSlice) Str() string {
	var sb strings.Builder
	var start, end, step string
	if is.Start != nil {
		start = str(is.Start)
	} else {
		start = ""
	}
	if is.End != nil {
		end = str(is.End)
	} else {
		end = ""
	}
	if is.Step != nil {
		step = str(is.Step)
	} else {
		step = ""
	}
	sb.WriteString(start)
	sb.WriteByte(':')
	sb.WriteString(end)
	sb.WriteByte(':')
	sb.WriteString(step)
	return sb.String()
}

type IndexExpression struct {
	Token tokens.Token
	Left,
	Index Expression
}

func (i IndexExpression) Line() int {
	return i.Token.Loc.Line
}

func (i IndexExpression) expressionNode() {}
func (i IndexExpression) TokenLiteral() string {
	return i.Token.Literal
}

func (i IndexExpression) Str() string {
	var sb strings.Builder
	sb.WriteString(i.Left.Str())
	if i.Index == nil {
		sb.WriteString("[]")
		return sb.String()
	}
	sb.WriteByte('[')
	sb.WriteString(i.Index.Str())
	sb.WriteByte(']')
	return sb.String()
}

type Map struct {
	Token tokens.Token
	Keys,
	Items []Expression
}

func (m Map) Line() int {
	return m.Token.Loc.Line
}

func (m Map) expressionNode() {}
func (m Map) TokenLiteral() string {
	return m.Token.Literal
}

func (m Map) Str() string {
	var sb strings.Builder
	sb.WriteString("{")
	for i := 0; i < len(m.Keys); i++ {
		sb.WriteString(str(m.Keys[i]))
		sb.WriteByte(':')
		sb.WriteString(str(m.Items[i]))
		if i == len(m.Keys)-1 {
		} else {
			sb.WriteString(", ")
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

func (mc MethodCall) Line() int {
	return mc.Token.Loc.Line
}

func (mc MethodCall) expressionNode() {}
func (mc MethodCall) TokenLiteral() string {
	return mc.Token.Literal
}

func (mc MethodCall) Str() string {
	var sb strings.Builder
	sb.WriteString(str(mc.Left))
	sb.WriteByte('.')
	for i, m := range mc.Methods {
		sb.WriteString(str(m))
		sb.WriteByte('(')
		for idx, arg := range mc.Arguments[i] {
			if idx != len(mc.Arguments[i])-1 {
				sb.WriteString(str(arg))
				sb.WriteByte(',')
			} else {
				sb.WriteString(str(arg))
			}
		}
		sb.WriteString(")")
		if i != len(mc.Methods)-1 {
			sb.WriteString(".")
		}
	}
	return sb.String()
}

type BreakExpr struct {
	Token tokens.Token
}

func (b BreakExpr) Line() int {
	return b.Token.Loc.Line
}

func (b BreakExpr) expressionNode() {}
func (b BreakExpr) TokenLiteral() string {
	return b.Token.Literal
}

func (b BreakExpr) Str() string {
	return b.Token.Literal
}
