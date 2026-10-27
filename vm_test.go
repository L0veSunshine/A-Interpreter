package main

import (
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/parser"
	vm2 "Interpreter/vm"
	"math"
	"strconv"
	"testing"
)

var exp = "9/2-9*8.102*2"

func TestAll(t *testing.T) {
	l := lexer.NewLexer(exp)
	p := parser.NewParser(l)
	nodes := p.Parse()
	if p.HasError() {
		t.Fatalf("parse error: %v", p.Errs())
	}
	comp := compiler.NewCompiler()
	comp.SetSymbol(p.SymTable)
	comp.Compile(nodes)
	if comp.HasError() {
		t.Fatalf("compile error: %v", comp.Errs())
	}
	vm := vm2.NewVM()
	if err := vm.Run(comp.ByteCode()); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	got, err := strconv.ParseFloat(vm.LastPop().Inspect(), 64)
	if err != nil {
		t.Fatalf("result is not a number: %s", vm.LastPop().Inspect())
	}
	if want := 4.5 - 9*8.102*2; math.Abs(got-want) > 1e-6 {
		t.Fatalf("want %v, got %v", want, got)
	}
}
