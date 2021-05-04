package main

import (
	"fmt"
	"testing"
)

var s = `10/2-9*8*2`

func TestParser_Parse(t *testing.T) {
	lex := NewLexer(s)
	p := NewParser(lex)
	ast := p.Parse()
	if !p.HasError() {
		fmt.Println(ast.Str())
	} else {
		fmt.Println(p.errs)
	}
	inter := NewExe()
	res := inter.visit(ast)
	fmt.Println(res.Inspect())
	//inter.GetGlobalTable()

	//fmt.Println(ast.ToString())
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
