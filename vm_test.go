package main

import (
	"fmt"
	"testing"
)

var exp = "9/2-9*8.102*2"

func TestAll(t *testing.T) {
	l := NewLexer(exp)
	p := NewParser(l)
	nodes := p.Parse()
	comp := NewCompiler()
	comp.Compile(nodes)
	vm := NewVM(comp.ByteCode())
	fmt.Println(comp.ByteCode().Instruction, comp.ByteCode().Constants)
	err := vm.Run()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(vm.stack, vm.sp)
}
