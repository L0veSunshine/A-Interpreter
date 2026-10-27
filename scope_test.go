package main

import (
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/parser"
	vm2 "Interpreter/vm"
	"testing"
)

func evalSrc(t *testing.T, src string) string {
	t.Helper()
	lex := lexer.NewLexer(src)
	p := parser.NewParser(lex)
	node := p.Parse()
	if p.HasError() {
		t.Fatalf("parse error: %v", p.Errs())
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.Compile(node)
	if c.HasError() {
		t.Fatalf("compile error: %v", c.Errs())
	}
	vm := vm2.NewVM()
	if err := vm.Run(c.ByteCode()); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	return vm.LastPop().Inspect()
}

func compileErrs(src string) []error {
	lex := lexer.NewLexer(src)
	p := parser.NewParser(lex)
	node := p.Parse()
	if p.HasError() {
		return p.Errs()
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.Compile(node)
	if c.HasError() {
		return c.Errs()
	}
	return nil
}

func TestBlockScopeShadowing(t *testing.T) {
	src := `var x=1
if (true){
	var x=2
	x=x+10
}
x`
	if got := evalSrc(t, src); got != "1" {
		t.Fatalf("outer x should stay 1, got %s", got)
	}
}

func TestBlockScopeSiblingsGetOwnSlots(t *testing.T) {
	src := `var s=0
if (true){
	var y=5
	s=s+y
}
if (true){
	var y=7
	s=s+y
}
s`
	if got := evalSrc(t, src); got != "12" {
		t.Fatalf("want 12, got %s", got)
	}
}

func TestBlockScopeInvisibleAfterBlock(t *testing.T) {
	src := `if (true){
	var y=5
}
y`
	if errs := compileErrs(src); errs == nil {
		t.Fatal("variable defined in a block must not be visible outside it")
	}
}

func TestBlockScopeForInitDoesNotLeak(t *testing.T) {
	src := `for(var i=0;i<3;i=i+1){
}
i`
	if errs := compileErrs(src); errs == nil {
		t.Fatal("loop variable must not leak out of the for statement")
	}
}

func TestBlockScopeLoopBodyVars(t *testing.T) {
	src := `var s=0
for(var i=0;i<100;i=i+1){
	var t=i*2
	s=s+t
}
s`
	if got := evalSrc(t, src); got != "9900" {
		t.Fatalf("want 9900, got %s", got)
	}
}

func TestBlockScopeShadowAllowsOuterUpdate(t *testing.T) {
	src := `var i=0
var s=0
for(;i<3;){
	var n=i
	s=s+n
	i=i+1
}
s`
	if got := evalSrc(t, src); got != "3" {
		t.Fatalf("want 3, got %s", got)
	}
}

func TestBlockScopeIfElseBranches(t *testing.T) {
	src := `var r=0
if (true){
	var q=1
	r=r+q
}else{
	var q=2
	r=r+q
}
r`
	if got := evalSrc(t, src); got != "1" {
		t.Fatalf("want 1, got %s", got)
	}
}

func TestBlockScopeDoesNotHideParams(t *testing.T) {
	src := `def f(n){
	if(true){
		var n=5
	}
	return n
}
f(3)`
	if got := evalSrc(t, src); got != "3" {
		t.Fatalf("param n should stay 3, got %s", got)
	}
}

func TestBlockScopeDuplicateDeclInSameScope(t *testing.T) {
	src := `var a=1
var a=2
a`
	if errs := compileErrs(src); errs == nil {
		t.Fatal("declaring the same name twice in one scope should fail")
	}
}
