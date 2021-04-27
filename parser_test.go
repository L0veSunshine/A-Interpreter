package main

import (
	"fmt"
	"testing"
)

var s = `9//2-9*8.102**2`

func TestParser_Parse(t *testing.T) {
	lex := NewLexer(s)
	fmt.Println(lex.Array())
	p := NewParser(lex)
	//fmt.Println(p.Parse(),p.HasError())
	ast := p.Parse()
	inter := New()
	res := inter.visit(ast)
	fmt.Println(res)
	fmt.Println(ast)
	//inter.GetGlobalTable()

	//fmt.Println(ast.ToString())
}

func BenchmarkParser_Parse(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex := NewLexer(s)
		p := NewParser(lex)
		ast := p.Parse()
		ast.String()
	}
}

func Test1(t *testing.T) {
	a := []int{1, 2, 3, 4}
	b := []int{3, 4, 5, 6}
	a = append(a, b...)
	fmt.Println(a)
}
