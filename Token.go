package main

import (
	"fmt"
	"strconv"
)

const (
	Integer      = "Integer"
	Real         = "Real"
	IntegerConst = "IntegerConst"
	RealConst    = "RealConst"
	Plus         = "Plus"
	Minus        = "Minus"
	Pow          = "Pow"
	Mul          = "Mul"
	IntegerDiv   = "IntegerDiv"
	FloatDiv     = "FloatDiv"
	LParen       = "LParen"
	RParen       = "RParen"
	ID           = "ID"
	Assign       = "Assign"
	Begin        = "Begin"
	End          = "End"
	Semi         = "Semi"
	Dot          = "Dot"
	Program      = "Program"
	Var          = "Var"
	Colon        = "Colon"
	Comma        = "Comma"
	EOF          = "EOF"
)

var Reserved = map[string]*Token{
	"PROGRAM": NToken(Program, Program),
	"VAR":     NToken(Var, Var),
	"DIV":     NToken(IntegerDiv, IntegerDiv),
	"INTEGER": NToken(Integer, Integer),
	"REAL":    NToken(Real, Real),
	"BEGIN":   NToken(Begin, Begin),
	"END":     NToken(End, End),
}

func NToken(Type string, Value interface{}) *Token {
	return &Token{
		Type:  Type,
		Value: Value,
	}
}

type Token struct {
	Type  string
	Value interface{}
}

func (t *Token) Str() string {
	s := fmt.Sprint(t.Value)
	return fmt.Sprintf("Token(%s%s%s)", t.Type, " ", strconv.Quote(s))
}

func (t *Token) IsEOF() bool {
	return t.Type == EOF
}
