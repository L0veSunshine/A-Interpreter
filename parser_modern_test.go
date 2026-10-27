package main

import (
	"Interpreter/lexer"
	"Interpreter/parser"
	"strings"
	"testing"
)

// newParserFor 是测试用的小工具：构造一个解析器。
func newParserFor(src string) *parser.Parser {
	return parser.NewParser(lexer.NewLexer(src))
}

// 重构后的解析器应当允许在容器内部换行（多行字面量/多行实参）。
func TestMultilineContainers(t *testing.T) {
	cases := map[string]string{
		"var a=[\n1,\n2,\n3\n]\nlen(a)": "3",
		`var m={
1:2,
3:4
}
len(m)`: "2",
		`def f(a,b){
	return a+b
}
f(
1,
2
)`: "3",
		// 括号内换行
		"var x=(1+\n2)\nx": "3",
		// 运算符后换行
		"var x=1+\n2\nx": "3",
		// 赋值右侧换行
		"var x=\n5\nx": "5",
	}
	for src, want := range cases {
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%q: want %s, got %s", src, want, got)
		}
	}
}

// 链式下标/调用的组合。
func TestChainedPostfix(t *testing.T) {
	cases := map[string]string{
		"var a=[[1,2],[3,4]]\na[1][0]":            "3",
		"var a=[1,2,3]\na[0:2][1]":                "2",
		"var s=\"hello\"\ns[1:4]":                 "ell",
		"var s=\"hello\"\ns[::2]":                 "hlo",
		"var s=\"hello\"\ns[::-1]":                "olleh",
		"var a=[1,2,3,4,5]\na[1:4]":               "[2, 3, 4]",
		"var a=[1,2,3]\nlen(a)":                   "3",
		`var s="a b c"` + "\nlen(s.split(\" \"))": "3",
	}
	for src, want := range cases {
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%q: want %s, got %s", src, want, got)
		}
	}
}

// 语句级的下标表达式（不是赋值）要继续可用。
func TestIndexExpressionAsStatement(t *testing.T) {
	src := `var a=[1,2,3]
a[1]`
	if got := evalSrc(t, src); got != "2" {
		t.Fatalf("want 2, got %s", got)
	}
}

// 错误信息必须带 line/col。
func TestParseErrorHasPosition(t *testing.T) {
	cases := []string{
		"var =1",
		"if(true){",
		"def f( {",
		"1+",
		"a[",
		"{1 2}",
	}
	for _, src := range cases {
		p := newParserFor(src)
		p.Parse()
		if !p.HasError() {
			t.Fatalf("%q: expected a parse error", src)
		}
		first := p.Errs()[0].Error()
		if !strings.Contains(first, "line ") || !strings.Contains(first, "col ") {
			t.Fatalf("%q: error should carry line/col, got %q", src, first)
		}
	}
}

// 一处错误不应该让解析器崩溃，也不应该无限报错。
func TestParseErrorRecovery(t *testing.T) {
	src := "var a=1\n@#$\nvar b=2\nb"
	p := newParserFor(src)
	p.Parse()
	if !p.HasError() {
		t.Fatal("expected parse errors")
	}
	if len(p.Errs()) > 100 {
		t.Fatalf("error count should be bounded, got %d", len(p.Errs()))
	}
}
