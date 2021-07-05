package main

import (
	"Interpreter/compiler"
	vm2 "Interpreter/vm"
	"fmt"
	"testing"
)

var exp = "9/2-9*8.102*2"

func TestAll(t *testing.T) {
	l := NewLexer(exp)
	p := NewParser(l)
	nodes := p.Parse()
	comp := compiler.NewCompiler()
	comp.Compile(nodes)
	vm := vm2.NewVM()
	fmt.Println(comp.ByteCode().Instruction, comp.ByteCode().Constants)
	err := vm.Run(comp.ByteCode())
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(vm.LastPop().Inspect())
}
