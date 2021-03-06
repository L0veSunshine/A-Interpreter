package tokens

import (
	"fmt"
	"strconv"
)

const (
	Int    = "Int"
	Float  = "Float"
	String = "String"

	Plus  = "Plus"  // +
	Minus = "Minus" // -
	Pow   = "Pow"   // **
	Mul   = "Mul"   // *
	Div   = "Div"   // /
	Mod   = "Mod"   // %

	IPlus  = "IPlus"  // +=
	IMinus = "IMinus" // -=
	IPow   = "IPow"   // **=
	IMul   = "IMul"   // *=
	IDiv   = "IDiv"   // /=
	IMod   = "IMod"   // %=

	Equal = "Equal" // ==
	NotEq = "NotEq" // !=
	LT    = "LT"    // <
	LTEq  = "LTEq"  // <=
	GT    = "GT"    // >
	GTEq  = "GTEq"  // >=

	And = "And"
	Or  = "Or"
	Not = "Not"

	LParen   = "LParen"   // (
	RParen   = "RParen"   // )
	LBRACE   = "LBRACE"   // {
	RBRACE   = "RBRACE"   // }
	LBRACKET = "LBRACKET" // [
	RBRACKET = "RBRACKET" // ]
	Var      = "Var"
	For      = "For"
	True     = "True"
	False    = "False"
	None     = "None"
	If       = "If"
	Else     = "Else"
	Break    = "Break"
	Func     = "Func"
	Return   = "Return"
	Assign   = "Assign"

	Ident = "Ident"

	Dot   = "Dot"   //.
	Colon = "Colon" //:
	Comma = "Comma" //,
	Semi  = "Semi"  //;
	LF    = "LF"

	EOF     = "EOF"
	Illegal = "Illegal"
)

var Reserved = map[string]string{
	"var":    Var,
	"for":    For,
	"def":    Func,
	"if":     If,
	"else":   Else,
	"return": Return,
	"true":   True,
	"false":  False,
	"and":    And,
	"or":     Or,
	"not":    Not,
	"none":   None,
	"break":  Break,
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
