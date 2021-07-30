package main

import (
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/object"
	"Interpreter/parser"
	vm2 "Interpreter/vm"
	"fmt"
	"testing"
)

var exp = "9/2-9*8.102*2"

func TestAll(t *testing.T) {
	l := lexer.NewLexer(exp)
	p := parser.NewParser(l)
	nodes := p.Parse()
	comp := compiler.NewCompiler()
	comp.Compile(nodes)
	vm := vm2.NewVM()
	fmt.Println(comp.ByteCode().Instruction, comp.ByteCode().Constants)
	err := vm.Run(comp.ByteCode())
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(object.Inspect(vm.LastPop()))
}
