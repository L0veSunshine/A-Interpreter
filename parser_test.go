package main

import (
	"fmt"
	"testing"
)

func TestParser_Parse(t *testing.T) {
	lex := NewLexer(s1)
	p := NewParser(lex)
	ast := p.Parse()
	//inter:=New()
	//res:=inter.visit(ast)
	//inter.GetGlobalTable()
	fmt.Println(ast.ToString())
}

func BenchmarkParser_Parse(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex := NewLexer(s1)
		p := NewParser(lex)
		ast := p.Parse()
		ast.ToString()
	}
}

func Test1(t *testing.T) {
	a := []int{1, 2, 3, 4}
	b := []int{3, 4, 5, 6}
	a = append(a, b...)
	fmt.Println(a)
}
