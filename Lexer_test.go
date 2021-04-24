package main

import (
	"fmt"
	"testing"
)

var s = `
 BEGIN
     BEGIN
         number := 2;
         a := number;
         b := 10 * a + 10 * number / 4;
         c := a - - b
     END;

     x := 11;
END.`

var s1 = `
PROGRAM Part10;
VAR
   number     : INTEGER;
   a, b, c, x : INTEGER;
   y          : REAL;

BEGIN {Part10}
   BEGIN
      number := 2;
      a := number;
      b := 10 * a + 10 * number DIV 4;
      c := a - - b
   END;
   x := 11;
   y := 20 / 7 + 3.14;
   { writeln('a = ', a); }
   { writeln('b = ', b); }
   { writeln('c = ', c); }
   { writeln('number = ', number); }
   { writeln('x = ', x); }
   { writeln('y = ', y); }
END.  {Part10}
`

func TestNewLexer(t *testing.T) {
	lex := NewLexer(s1)
	for i := 0; i < 50; i++ {
		fmt.Println(lex.NextToken())
	}
}

func BenchmarkLexer_Array(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex := NewLexer(s1)
		lex.Array()
	}
}
