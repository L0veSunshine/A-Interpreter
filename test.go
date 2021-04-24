package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	run()
	//expr := "1-2**(3*0.5)"
	//lex := NewLexer(expr)
	//p := NewParser(lex)
	//r := p.Parse()
	//fmt.Println(r.ToString())
	//res := Exec(r)
	//fmt.Println(res)
}

func run() {
	defer func() {
		if r := recover(); r != nil {
			run()
		}
	}()
	var expr []byte
	for {
		fmt.Print(">>>")
		reader := bufio.NewReader(os.Stdin)
		expr, _, _ = reader.ReadLine()
		if string(expr) == "exit" || string(expr) == "e" {
			break
		}
		lex := NewLexer(string(expr))
		p := NewParser(lex)
		r := p.Parse()
		fmt.Println(r.ToString())
		res := Exec(r)
		fmt.Println(res)
	}
}
