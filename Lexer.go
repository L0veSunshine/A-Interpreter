package main

import (
	"Interpreter/utils"
	"fmt"
	"strconv"
)

type Lexer struct {
	rs  []rune
	pos int
	cur *utils.Char
	err error
}

func NewLexer(text string) *Lexer {
	l := &Lexer{
		rs: []rune(text),
	}
	l.cur = utils.Code(l.rs[l.pos])
	return l
}

func (l Lexer) Array() []*Token {
	var tokens []*Token
	first := l.NextToken()
	for ; !first.IsEOF(); first = l.NextToken() {
		tokens = append(tokens, first)
	}
	return tokens
}

func (l *Lexer) advance(step int) {
	l.pos += step
	if l.pos >= len(l.rs) {
		l.cur = utils.Code(0)
	} else {
		l.cur = utils.Code(l.rs[l.pos])
	}
}

func (l *Lexer) skipWhitespace() {
	for l.cur.IsWhitespace() && !l.cur.IsNull() {
		l.advance(1)
	}
}

func (l *Lexer) skipComment() {
	for !l.cur.Equal("}") {
		l.advance(1)
	}
	l.advance(1)
}

func (l *Lexer) peek() *utils.Char {
	peekPos := l.pos + 1
	if peekPos >= len(l.rs) {
		return utils.Code(0)
	} else {
		return utils.Code(l.rs[peekPos])
	}
}

func (l *Lexer) number() *Token {
	var value []rune
	for !l.cur.IsNull() && l.cur.IsDigital() {
		value = append(value, l.cur.Rune())
		l.advance(1)
	}
	if l.cur.Equal(".") {
		value = append(value, l.cur.Rune())
		l.advance(1)
		for !l.cur.IsNull() && l.cur.IsDigital() {
			value = append(value, l.cur.Rune())
			l.advance(1)
		}
		floatValue, err := strconv.ParseFloat(string(value), 64)
		if err != nil {
			fmt.Println(err)
		}
		return NToken(RealConst, floatValue)
	} else {
		intValue, err := strconv.Atoi(string(value))
		if err != nil {
			fmt.Println(err)
		}
		return NToken(IntegerConst, intValue)
	}

}

func (l *Lexer) id() *Token {
	var value []rune
	for !l.cur.IsNull() && l.cur.IsAlNum() {
		value = append(value, l.cur.Rune())
		l.advance(1)
	}
	valueStr := string(value)
	if cur, ok := Reserved[valueStr]; ok {
		return cur
	}
	t := NToken(ID, valueStr)
	return t
}

func (l *Lexer) NextToken() *Token {
LOOP:
	l.skipWhitespace()
	switch {
	case l.cur.Equal("{"):
		l.advance(1)
		l.skipComment()
		goto LOOP
	case l.cur.IsAlpha():
		return l.id()
	case l.cur.IsDigital():
		return l.number()
	case l.cur.Equal(":") && l.peek().Equal("="):
		l.advance(2)
		return NToken(Assign, ":=")
	case l.cur.Equal(";"):
		l.advance(1)
		return NToken(Semi, ";")
	case l.cur.Equal(":"):
		l.advance(1)
		return NToken(Colon, ":")
	case l.cur.Equal(","):
		l.advance(1)
		return NToken(Comma, ",")
	case l.cur.Equal("+"):
		l.advance(1)
		return NToken(Plus, "+")
	case l.cur.Equal("-"):
		l.advance(1)
		return NToken(Minus, "-")
	case l.cur.Equal("*"):
		p := l.peek()
		if p.Equal("*") {
			l.advance(2)
			return NToken(Pow, "**")
		}
		l.advance(1)
		return NToken(Mul, "*")
	case l.cur.Equal("/"):
		l.advance(1)
		return NToken(FloatDiv, "/")
	case l.cur.Equal("("):
		l.advance(1)
		return NToken(LParen, "(")
	case l.cur.Equal(")"):
		l.advance(1)
		return NToken(RParen, ")")
	case l.cur.Equal("."):
		l.advance(1)
		return NToken(Dot, ".")
	case l.cur.IsNull():
		l.advance(1)
		return NToken(EOF, nil)
	}
	inf := "illegal token " + l.cur.Quote() + " at loc " + strconv.Itoa(l.pos+1)
	panic(inf)
}
