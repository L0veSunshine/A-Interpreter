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
