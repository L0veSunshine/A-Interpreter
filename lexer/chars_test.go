package lexer

import "testing"

func TestCharEOFIsNotNUL(t *testing.T) {
	// 关键：NUL 是合法源码字符，不能被当成 EOF，
	// 否则 NUL 之后的代码会被静默丢弃。
	if !EOF.IsEOF() {
		t.Fatal("EOF sentinel must report IsEOF")
	}
	if Char(0).IsEOF() {
		t.Fatal("NUL character must not be treated as EOF")
	}
	if !Char(0).Is(0) {
		t.Fatal("NUL should compare equal to rune 0")
	}
	if EOF.Rune() != -1 {
		t.Fatalf("EOF rune should be -1, got %d", EOF.Rune())
	}
}

func TestCharClassify(t *testing.T) {
	digits := []rune("0123456789")
	for _, r := range digits {
		if !Char(r).IsDigit() {
			t.Fatalf("%q should be a digit", r)
		}
	}
	for _, r := range []rune("abcXYZ_") {
		if Char(r).IsDigit() {
			t.Fatalf("%q should not be a digit", r)
		}
		if !Char(r).IsIdentStart() {
			t.Fatalf("%q should start an identifier", r)
		}
	}
	for _, r := range []rune("aZ0_") {
		if !Char(r).IsIdentPart() {
			t.Fatalf("%q should continue an identifier", r)
		}
	}
	for _, r := range []rune("+-*/%(){}[],:.;!<>=#\"'@$") {
		if Char(r).IsIdentStart() || Char(r).IsIdentPart() {
			t.Fatalf("%q must not be part of an identifier", r)
		}
	}
}

func TestCharUnicodeIdentifiers(t *testing.T) {
	for _, r := range []rune("变量名πλ日本語") {
		if !Char(r).IsIdentStart() {
			t.Fatalf("%q should be a valid identifier start", r)
		}
		if !Char(r).IsIdentPart() {
			t.Fatalf("%q should be a valid identifier part", r)
		}
	}
	// 全角数字不算数字字面量的数字
	if Char('１').IsDigit() {
		t.Fatal("full-width digit must not count as a numeric literal digit")
	}
}

func TestCharSpaceAndNewline(t *testing.T) {
	spaces := []rune{' ', '\t', '\v', '\f', 0x85, 0xA0}
	for _, r := range spaces {
		if !Char(r).IsSpace() {
			t.Fatalf("%q should be whitespace", r)
		}
		if Char(r).IsNewline() {
			t.Fatalf("%q should not be a newline", r)
		}
	}
	if !Char('\n').IsNewline() {
		t.Fatal("\\n should be a newline")
	}
	if Char('\n').IsSpace() {
		t.Fatal("newline must not count as plain whitespace")
	}
	if !Char('\r').IsSpace() {
		t.Fatal("\\r should be whitespace")
	}
	if EOF.IsSpace() || EOF.IsNewline() || EOF.IsDigit() || EOF.IsIdentStart() {
		t.Fatal("EOF must not classify as any character class")
	}
}

func TestCharHexAndQuotes(t *testing.T) {
	for _, r := range []rune("0123456789abcdefABCDEF") {
		if !Char(r).IsHexDigit() {
			t.Fatalf("%q should be a hex digit", r)
		}
	}
	for _, r := range []rune("gGxX_ ") {
		if Char(r).IsHexDigit() {
			t.Fatalf("%q should not be a hex digit", r)
		}
	}
	for _, r := range []rune{'"', '\''} {
		if !Char(r).IsQuote() {
			t.Fatalf("%q should be a quote", r)
		}
	}
	if Char('`').IsQuote() {
		t.Fatal("backtick should not be a supported quote")
	}
}

func TestCharDigitValue(t *testing.T) {
	cases := []struct {
		r    rune
		base int
		want int
	}{
		{'0', 10, 0},
		{'9', 10, 9},
		{'9', 8, -1},
		{'7', 8, 7},
		{'f', 16, 15},
		{'F', 16, 15},
		{'2', 2, -1},
		{'1', 2, 1},
		{'z', 16, -1},
		{'_', 10, -1},
	}
	for _, tc := range cases {
		if got := Char(tc.r).digitValue(tc.base); got != tc.want {
			t.Fatalf("%q base %d: want %d, got %d", tc.r, tc.base, tc.want, got)
		}
	}
}

func TestCharStringAndQuote(t *testing.T) {
	if got := Char('a').String(); got != "a" {
		t.Fatalf("String() = %q", got)
	}
	if got := EOF.String(); got != "<EOF>" {
		t.Fatalf("EOF.String() = %q", got)
	}
	if got := Char('\n').Quote(); got != `'\n'` {
		t.Fatalf("quote of newline = %q", got)
	}
	if got := EOF.Quote(); got != "<EOF>" {
		t.Fatalf("EOF.Quote() = %q", got)
	}
}
