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
	Ident     tokens.Token // ident
	Statement Expression
}

func (as AssignStatement) StatementNode() {}
func (as AssignStatement) TokenLiteral() string {
	return as.Ident.Literal
}
func (as AssignStatement) Str() string {
	var sb strings.Builder
	sb.WriteString(as.Ident.Literal + " = ")
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
