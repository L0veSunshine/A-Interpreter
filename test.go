package main

import (
	"fmt"
	"unsafe"
)

//func run() {
//	defer func() {
//		if r := recover(); r != nil {
//			run()
//		}
//	}()
//	var expr []byte
//	for {
//		fmt.Print(">>>")
//		reader := bufio.NewReader(os.Stdin)
//		expr, _, _ = reader.ReadLine()
//		if string(expr) == "exit" || string(expr) == "e" {
//			break
//		}
//		lex := NewLexer(string(expr))
//		p := NewParser(lex)
//		r := p.Parse()
//		fmt.Println(r.ToString())
//		res := Exec(r)
//		fmt.Println(res)
//	}
//}
type A struct {
	X int
	Y int
	Z int
}

type B struct {
	A, B, C int
}

func main() {
	a := new(A)
	a.X = 1
	a.Y = 2
	b := new(B)
	b.A = 1
	b.B = 2
	p := unsafe.Pointer(&b)
	co := *(*A)(p)
	fmt.Println(b.B, co.Z, co.Y)
}
