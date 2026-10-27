// 临时命令行入口：读入一个源码文件并执行，用于端到端计时对比。
package main

import (
	"fmt"
	"os"

	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/parser"
	"Interpreter/vm"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: xlang <file>")
		os.Exit(2)
	}
	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p := parser.NewParser(lexer.NewLexer(string(src)))
	node := p.Parse()
	if p.HasError() {
		fmt.Fprintln(os.Stderr, p.Errs())
		os.Exit(1)
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.Compile(node)
	if c.HasError() {
		fmt.Fprintln(os.Stderr, c.Errs())
		os.Exit(1)
	}
	m := vm.NewVM()
	if err := m.Run(c.ByteCode()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(m.LastPop().Inspect())
}
