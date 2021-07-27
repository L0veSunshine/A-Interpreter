package lexer

import (
	"fmt"
	"testing"
)

var eg = `var t=10/2-9*8*2
print(t)`

func TestNewLexer(t *testing.T) {
	lex := NewLexer(eg)
	for i := 0; i < 50; i++ {
		fmt.Println(lex.NextToken())
	}
}

func BenchmarkLexer_Array(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex := NewLexer(eg)
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
	fmt.Println(l.Errs())
}
