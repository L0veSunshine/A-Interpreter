package main

import (
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/parser"
	"Interpreter/vm"
	"fmt"
	"sync"
)

var c *compiler.Compiler
var ones sync.Once

func singleComp() *compiler.Compiler {
	ones.Do(func() {
		c = compiler.NewCompiler()
	})
	return c
}

func run(code string) string {
	lex := lexer.NewLexer(code)
	parser := parser.NewParser(lex)
	ast := parser.Parse()
	if parser.HasError() {
		return fmt.Sprint(parser.Errors.Errs())
	}
	comp := singleComp()
	comp.Compile(ast)
	fmt.Println(comp.ByteCode())
	VM := vm.NewVM()
	if err := VM.Run(comp.ByteCode()); err != nil {
		return fmt.Sprint(err)
	}
	if VM.LastPop() != nil {
		return VM.LastPop().Inspect()
	}
	return ""
}

func main() {
	//var res string
	//if len(os.Args) > 1 {
	//	fArg := os.Args[1]
	//	f, e := os.Open(fArg)
	//	if e != nil {
	//		fmt.Println(e)
	//		return
	//	}
	//	bytes, err := io.ReadAll(f)
	//	if err != nil {
	//		fmt.Println(err)
	//		return
	//	}
	//	res = run(string(bytes))
	//	fmt.Print(res)
	//} else {
	//	var expr []byte
	//	for {
	//		fmt.Print(">>>")
	//		reader := bufio.NewReader(os.Stdin)
	//		expr, _, _ = reader.ReadLine()
	//		if string(expr) == "exit" {
	//			fmt.Print("Bye!")
	//			break
	//		}
	//		res = run(string(expr))
	//		fmt.Println(res)
	//	}
	//}
}
