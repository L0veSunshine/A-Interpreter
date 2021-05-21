package tokens

import (
	"fmt"
	"strconv"
)

const (
	Number = "Number"
	Ident  = "Ident"

	Plus  = "Plus"  // +
	Minus = "Minus" // -
	Pow   = "Pow"   // **
	Mul   = "Mul"   // *
	Div   = "Div"   // /
	Floor = "Floor" // //

	Equal = "Equal" // ==
	NotEq = "NotEq" // !=
	LT    = "LT"    // <
	LTEq  = "LTEq"  // <=
	GT    = "GT"    // >
	GTEq  = "GTEq"  // >=

	And = "And"
	Or  = "Or"
	Not = "Not"

	LParen = "LParen" // (
	RParen = "RParen" // )
	LBRACE = "LBRACE" // {
	RBRACE = "RBRACE" // }
	Var    = "Var"
	For    = "For"
	True   = "True"
	False  = "False"
	If     = "If"
	Else   = "Else"
	Func   = "Func"
	Return = "Return"
	Assign = "Assign"

	String = "String"

	Dot   = "Dot"   //.
	Colon = "Colon" //:
	Comma = "Comma" //,
	LF    = "LF"

	EOF     = "EOF"
	Illegal = "Illegal"
)

var Reserved = map[string]string{
	"var":    Var,
	"for":    For,
	"fn":     Func,
	"if":     If,
	"else":   Else,
	"return": Return,
	"true":   True,
	"false":  False,
	"and":    And,
	"or":     Or,
	"not":    Not,
}

type Locate struct {
	Column, Line int
}

func NToken(Type string, Value string, loc *Locate) *Token {
	return &Token{
		Type:    Type,
		Literal: Value,
		Loc:     *loc,
	}
}

type Token struct {
	Type    string
	Literal string
	Loc     Locate
}

func (t *Token) Str() string {
	return fmt.Sprintf("Token(%s%s%s) at col%d, line%d.", t.Type, " ", t.Quote(),
		t.Loc.Column, t.Loc.Line)
}

func (t *Token) IsLF() bool {
	return t.Type == LF
}

func (t *Token) IsEOF() bool {
	return t.Type == EOF
}

func (t *Token) IsIllegal() bool {
	return t.Type == Illegal
}

func (t *Token) Quote() string {
	return strconv.Quote(t.Literal)
}
