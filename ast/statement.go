package ast

import (
	"Interpreter/tokens"
	"strings"
)

type Program struct {
	Statements []Statement
}

func (p Program) StatementNode() {}
func (p Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	} else {
		return ""
	}
}

func (p Program) Str() string {
	var sb strings.Builder
	for idx, s := range p.Statements {
		if idx != len(p.Statements)-1 {
			sb.WriteString(s.Str() + "\n")
		} else {
			sb.WriteString(s.Str())
		}
	}
	return sb.String()
}

type VarStatement struct {
	Token  tokens.Token // Var tokens
	Indent IdentNode
	Value  Expression
}

func (vs VarStatement) StatementNode() {}
func (vs VarStatement) TokenLiteral() string {
	return vs.Token.Literal
}

func (vs VarStatement) Str() string {
	var sb strings.Builder
	sb.WriteString(vs.TokenLiteral() + " ")
	sb.WriteString(vs.Indent.Str() + " = ")
	if vs.Value != nil {
		sb.WriteString(vs.Value.Str())
	}
	return sb.String()
}

type VarMethodCall struct {
	Token  tokens.Token // Var tokens
	Indent IdentNode
	Value  Expression
}

func (v VarMethodCall) StatementNode() {}
func (v VarMethodCall) TokenLiteral() string {
	return v.Token.Literal
}

func (v VarMethodCall) Str() string {
	var sb strings.Builder
	sb.WriteString(v.TokenLiteral() + " ")
	sb.WriteString(v.Indent.Str() + " = ")
	if v.Value != nil {
		sb.WriteString(v.Value.Str() + " <Method>")
	}
	return sb.String()
}

type MethodCallStmt struct {
	Token tokens.Token
	Call  Expression
}

func (m MethodCallStmt) StatementNode() {}
func (m MethodCallStmt) TokenLiteral() string {
	return m.Token.Literal
}

func (m MethodCallStmt) Str() string {
	return m.Call.Str()
}

type ReturnStatement struct {
	Token     tokens.Token // Return tokens
	ReturnVal Expression
}

func (rs ReturnStatement) StatementNode() {}
func (rs ReturnStatement) TokenLiteral() string {
	return rs.Token.Literal
}

func (rs ReturnStatement) Str() string {
	var sb strings.Builder
	sb.WriteString(rs.TokenLiteral() + " ")
	if rs.ReturnVal != nil {
		sb.WriteString(rs.ReturnVal.Str())
	}
	return sb.String()
}

type AssignStatement struct {
	Ident      tokens.Token // ident
	Identifier IdentNode
	Statement  Expression
}

func (as AssignStatement) StatementNode() {}
func (as AssignStatement) TokenLiteral() string {
	return as.Ident.Literal
}
func (as AssignStatement) Str() string {
	var sb strings.Builder
	sb.WriteString("assign: " + as.Identifier.Value + " = ")
	sb.WriteString(as.Statement.Str())
	return sb.String()
}

type ExprStatement struct {
	Expression Expression
}

func (es ExprStatement) StatementNode() {}
func (es ExprStatement) TokenLiteral() string {
	return es.Expression.TokenLiteral()
}

func (es ExprStatement) Str() string {
	if es.Expression != nil {
		return es.Expression.Str()
	}
	return ""
}

type FuncStatement struct {
	Expression Expression
}

func (fs FuncStatement) StatementNode() {}
func (fs FuncStatement) TokenLiteral() string {
	return fs.Expression.TokenLiteral()
}

func (fs FuncStatement) Str() string {
	if fs.Expression != nil {
		return fs.Expression.Str()
	}
	return ""
}

type BlockStatement struct {
	Token      tokens.Token
	Statements []Statement
}

func (bs BlockStatement) StatementNode() {}
func (bs BlockStatement) TokenLiteral() string {
	return bs.Token.Literal
}

func (bs BlockStatement) Str() string {
	var sb strings.Builder
	sb.WriteString("Stmts:{")
	for i, s := range bs.Statements {
		sb.WriteString(s.Str())
		if i != len(bs.Statements)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("}")
	return sb.String()
}

type ExpressionAssign struct {
	Token         tokens.Token
	Old, Key, New Expression
}

func (ea ExpressionAssign) StatementNode() {}
func (ea ExpressionAssign) TokenLiteral() string {
	return ea.Token.Literal
}

func (ea ExpressionAssign) Str() string {
	return ea.Old.Str() + "[" + ea.Key.Str() + "] = " + ea.New.Str()
}
