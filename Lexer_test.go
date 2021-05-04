package main

import (
	"fmt"
	"testing"
)

func TestNewLexer(t *testing.T) {
	lex := NewLexer(s)
	for i := 0; i < 50; i++ {
		fmt.Println(lex.NextToken())
	}
}

func BenchmarkLexer_Array(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex := NewLexer(s)
		lex.Array()
	}
}

func TestLexer_str(t *testing.T) {
	s := `#"hello",hx=123+5
# a=12,(b=}123,a=b
var a=1
if (a==1){
return a+1}else{
return abv !=  !  >=  <= ==}`
	l := NewLexer(s)
	fmt.Println(l.Array())
	fmt.Println(l.errs)
}
