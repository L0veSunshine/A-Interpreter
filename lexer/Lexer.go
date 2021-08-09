package lexer

import (
	"Interpreter/errors"
	"Interpreter/tokens"
)

type Lexer struct {
	rs  []rune
	pos int
	Loc *tokens.Locate
	cur *Char
	*errors.Errors
}

func NewLexer(text string) *Lexer {
	l := &Lexer{
		rs:     []rune(text),
		Errors: errors.NewErr(),
		Loc: &tokens.Locate{
			Column: 1,
			Line:   1,
		},
	}
	l.cur = Code(l.rs[l.pos])
	return l
}

func (l Lexer) Array() []tokens.Token {
	var ts []tokens.Token
	first := l.NextToken()
	for ; !first.IsEOF(); first = l.NextToken() {
		ts = append(ts, *first)
	}

	return ts
}

func (l *Lexer) advance(step int) {
	l.pos += step
	l.Loc.Column += step
	if l.pos >= len(l.rs) {
		l.cur = Code(0)
	} else {
		if l.cur.Equal("\n") {
			l.Loc.Column = 1
			l.Loc.Line += 1
		}
		l.cur = Code(l.rs[l.pos])
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

func (l *Lexer) peek() *Char {
	peekPos := l.pos + 1
	if peekPos >= len(l.rs) {
		return Code(0)
	} else {
		return Code(l.rs[peekPos])
	}
}

func (l *Lexer) number() *tokens.Token {
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
		return tokens.NToken(tokens.Float, string(value), l.Loc)
	} else {
		return tokens.NToken(tokens.Int, string(value), l.Loc)
	}
}

func (l *Lexer) id() *tokens.Token {
	var value []rune
	for !l.cur.IsNull() && l.cur.IsAlNum() {
		value = append(value, l.cur.Rune())
		l.advance(1)
	}
	valueStr := string(value)
	if cur, ok := tokens.Reserved[valueStr]; ok {
		return tokens.NToken(cur, cur, l.Loc)
	}
	t := tokens.NToken(tokens.Ident, valueStr, l.Loc)
	return t
}

func (l *Lexer) string() *tokens.Token {
	l.advance(1) //skip "
	var rs []rune
	for !l.cur.Equal(`"`) && !l.cur.IsNull() {
		rs = append(rs, l.cur.Rune())
		l.advance(1)
	}
	l.advance(1) //skip "
	return tokens.NToken(tokens.String, string(rs), l.Loc)
}

func (l *Lexer) illegal() *tokens.Token {
	var value []rune
	for !l.cur.IsNull() && !l.cur.IsWhitespace() {
		value = append(value, l.cur.Rune())
		l.advance(1)
	}
	l.NewErrorF("Illegal tokens %s at col%d, line%d.",
		string(value), l.Loc.Column, l.Loc.Line)
	return tokens.NToken(tokens.Illegal, string(value), l.Loc)
}

func (l *Lexer) NextToken() *tokens.Token {
LOOP:
	l.skipWhitespace()
	loc := l.Loc
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
			return tokens.NToken(tokens.Equal, "==", loc)
		}
		l.advance(1)
		return tokens.NToken(tokens.Assign, "=", loc)
	case l.cur.Equal("!"):
		if l.peek().Equal("=") {
			l.advance(2)
			return tokens.NToken(tokens.NotEq, "!=", loc)
		}
	case l.cur.Equal("<"):
		if l.peek().Equal("=") {
			l.advance(2)
			return tokens.NToken(tokens.LTEq, "<=", loc)
		}
		l.advance(1)
		return tokens.NToken(tokens.LT, "<", loc)
	case l.cur.Equal(">"):
		if l.peek().Equal("=") {
			l.advance(2)
			return tokens.NToken(tokens.GTEq, ">=", loc)
		}
		l.advance(1)
		return tokens.NToken(tokens.GT, ">", loc)
	case l.cur.Equal(":"):
		l.advance(1)
		return tokens.NToken(tokens.Colon, ":", loc)
	case l.cur.Equal(","):
		l.advance(1)
		return tokens.NToken(tokens.Comma, ",", loc)
	case l.cur.Equal("+"):
		l.advance(1)
		return tokens.NToken(tokens.Plus, "+", loc)
	case l.cur.Equal("-"):
		l.advance(1)
		return tokens.NToken(tokens.Minus, "-", loc)
	case l.cur.Equal("*"):
		p := l.peek()
		if p.Equal("*") {
			l.advance(2)
			return tokens.NToken(tokens.Pow, "**", loc)
		}
		l.advance(1)
		return tokens.NToken(tokens.Mul, "*", loc)
	case l.cur.Equal("/"):
		l.advance(1)
		return tokens.NToken(tokens.Div, "/", loc)
	case l.cur.Equal("%"):
		l.advance(1)
		return tokens.NToken(tokens.Mod, "%", loc)
	case l.cur.Equal(`"`):
		return l.string()
	case l.cur.Equal("("):
		l.advance(1)
		return tokens.NToken(tokens.LParen, "(", loc)
	case l.cur.Equal("["):
		l.advance(1)
		return tokens.NToken(tokens.LBRACKET, "[", loc)
	case l.cur.Equal("]"):
		l.advance(1)
		return tokens.NToken(tokens.RBRACKET, "]", loc)
	case l.cur.Equal(")"):
		l.advance(1)
		return tokens.NToken(tokens.RParen, ")", loc)
	case l.cur.Equal("{"):
		l.advance(1)
		return tokens.NToken(tokens.LBRACE, "{", loc)
	case l.cur.Equal("}"):
		l.advance(1)
		return tokens.NToken(tokens.RBRACE, "}", loc)
	case l.cur.Equal("."):
		l.advance(1)
		if l.cur.IsDigital() {
			var value = []rune("0.")
			for !l.cur.IsNull() && l.cur.IsDigital() {
				value = append(value, l.cur.Rune())
				l.advance(1)
			}
			return tokens.NToken(tokens.Float, string(value), l.Loc)
		}
		return tokens.NToken(tokens.Dot, ".", loc)
	case l.cur.Equal("\n"):
		l.advance(1)
		return tokens.NToken(tokens.LF, "LF", loc)
	case l.cur.IsNull():
		l.advance(1)
		return tokens.NToken(tokens.EOF, "", loc)
	}
	return l.illegal()
}
