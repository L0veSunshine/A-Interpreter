package main

import (
	"fmt"
	"strconv"
)

const (
	Number     = "Number"
	Plus       = "Plus"
	Minus      = "Minus"
	Pow        = "Pow"
	Mul        = "Mul"
	Div        = "Div"
	Floor      = "Floor"
	Equal      = "Equal"
	LParen     = "LParen"
	RParen     = "RParen"
	Var        = "Var"
	Assign     = "Assign"
	Dot        = "Dot"
	Identifier = "Identifier"
	Semi       = "Semi"
	Colon      = "Colon"
	Comma      = "Comma"
	EOF        = "EOF"
	Illegal    = "Illegal"
)

var Reserved = map[string]string{
	"var": Var,
}

type Locate struct {
	Column, Line int
}

func NToken(Type string, Value interface{}, loc *Locate) *Token {
	s := fmt.Sprint(Value)
	return &Token{
		Type:    Type,
		Literal: s,
		Loc:     *loc,
	}
}

type Token struct {
	Type    string
	Literal string
	Loc     Locate
}

func (t *Token) Str() string {
	return fmt.Sprintf("Token(%s%s%s) at col%d, line%d.", t.Type, " ", strconv.Quote(t.Literal),
		t.Loc.Column, t.Loc.Line)
}

func (t *Token) IsEOF() bool {
	return t.Type == EOF
}

func (t *Token) IsIllegal() bool {
	return t.Type == Illegal
}
