package main

import (
	"fmt"
	"testing"
)

var s = `10/2-9*8*2`
var s1 = `{var a=1
var b=a+3
b=b+2*a
c=-1+2
d=1<=2
return 6*a}
`
var s2 = `if (a<=3)
{a=a+3}`

func TestParser_Parse(t *testing.T) {
	lex := NewLexer(s2)
	p := NewParser(lex)
	ast := p.parseIfExpression()
	if !p.HasError() {
		fmt.Println(ast.Str())
	} else {
		fmt.Println(p.errs, len(p.errs))
	}
}

func BenchmarkParser_Parse(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	inter := NewExe()
	for i := 0; i < b.N; i++ {
		lex := NewLexer(s)
		p := NewParser(lex)
		ast := p.Parse()
		inter.visit(ast)
	}
}

func Test1(t *testing.T) {
	a := []int{1, 2, 3, 4}
	b := []int{3, 4, 5, 6}
	a = append(a, b...)
	fmt.Println(a)
}
