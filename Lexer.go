package main

import (
	"Interpreter/utils"
	"strconv"
)

type Lexer struct {
	rs  []rune
	pos int
	loc *Locate
	cur *utils.Char
	*Errors
}

func NewLexer(text string) *Lexer {
	l := &Lexer{
		rs:     []rune(text),
		Errors: NewErr(),
		loc: &Locate{
			Column: 1,
			Line:   1,
		},
	}
	l.cur = utils.Code(l.rs[l.pos])
	return l
}

func (l Lexer) Array() []Token {
	var tokens []Token
	first := l.NextToken()
	for ; !first.IsEOF(); first = l.NextToken() {
		tokens = append(tokens, *first)
	}

	return tokens
}

func (l *Lexer) advance(step int) {
	l.pos += step
	l.loc.Column += step
	if l.pos >= len(l.rs) {
		l.cur = utils.Code(0)
	} else {
		if l.cur.Equal("\n") {
			l.loc.Column = 1
			l.loc.Line += 1
		}
		l.cur = utils.Code(l.rs[l.pos])
	}
}

func (l *Lexer) skipWhitespace() {
	for l.cur.IsWhitespace() && !l.cur.IsNull() {
		l.advance(1)
	}
}

func (l *Lexer) skipComment() {
	for !l.cur.Equal("\n") && !l.cur.IsNull() {
		l.advance(1)
	}
	// 跳过\n
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
			l.Push(err)
		}
		return NToken(Number, floatValue, l.loc)
	} else {
		intValue, err := strconv.Atoi(string(value))
		if err != nil {
			l.Push(err)
		}
		return NToken(Number, intValue, l.loc)
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
		return NToken(cur, cur, l.loc)
	}
	t := NToken(Identifier, valueStr, l.loc)
	return t
}

func (l *Lexer) illegal() *Token {
	var value []rune
	for !l.cur.IsNull() && !l.cur.IsWhitespace() {
		value = append(value, l.cur.Rune())
		l.advance(1)
	}
	l.NewErrorF("Illegal tokens %s at col%d, line%d.",
		string(value), l.loc.Column, l.loc.Line)
	return NToken(Illegal, string(value), l.loc)
}

func (l *Lexer) NextToken() *Token {
LOOP:
	l.skipWhitespace()
	loc := l.loc
	switch {
	case l.cur.Equal("#"):
		l.advance(1)
		l.skipComment()
		goto LOOP
	case l.cur.IsAlpha():
		return l.id()
	case l.cur.IsDigital():
		return l.number()
	case l.cur.Equal("="):
		if l.peek().Equal("=") {
			l.advance(2)
			return NToken(Equal, "==", loc)
		}
		l.advance(1)
		return NToken(Assign, "=", loc)
	case l.cur.Equal(";"):
		l.advance(1)
		return NToken(Semi, ";", loc)
	case l.cur.Equal(":"):
		l.advance(1)
		return NToken(Colon, ":", loc)
	case l.cur.Equal(","):
		l.advance(1)
		return NToken(Comma, ",", loc)
	case l.cur.Equal("+"):
		l.advance(1)
		return NToken(Plus, "+", loc)
	case l.cur.Equal("-"):
		l.advance(1)
		return NToken(Minus, "-", loc)
	case l.cur.Equal("*"):
		p := l.peek()
		if p.Equal("*") {
			l.advance(2)
			return NToken(Pow, "**", loc)
		}
		l.advance(1)
		return NToken(Mul, "*", loc)
	case l.cur.Equal("/"):
		if l.peek().Equal("/") {
			l.advance(2)
			return NToken(Floor, "//", loc)
		}
		l.advance(1)
		return NToken(Div, "/", loc)
	case l.cur.Equal("("):
		l.advance(1)
		return NToken(LParen, "(", loc)
	case l.cur.Equal(")"):
		l.advance(1)
		return NToken(RParen, ")", loc)
	case l.cur.Equal("."):
		l.advance(1)
		return NToken(Dot, ".", loc)
	case l.cur.IsNull():
		l.advance(1)
		return NToken(EOF, nil, loc)
	}
	return l.illegal()
}
