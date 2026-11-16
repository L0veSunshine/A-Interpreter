package main

import (
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/parser"
	vm2 "Interpreter/vm"
	"fmt"
	"strings"
	"testing"
)

func evalOrErr(t *testing.T, src string) (string, error) {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(src))
	node := p.Parse()
	if p.HasError() {
		return "", fmt.Errorf("parse: %v", p.Errs())
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.Compile(node)
	if c.HasError() {
		return "", fmt.Errorf("compile: %v", c.Errs())
	}
	m := vm2.NewVM()
	if err := m.Run(c.ByteCode()); err != nil {
		return "", err
	}
	return m.LastPop().Inspect(), nil
}

func expect(t *testing.T, src, want string) {
	t.Helper()
	got, err := evalOrErr(t, src)
	if err != nil {
		t.Fatalf("%q => error: %v", src, err)
	}
	if got != want {
		t.Fatalf("%q => want %s, got %s", src, want, got)
	}
}

func expectErr(t *testing.T, src, wantPart string) {
	t.Helper()
	_, err := evalOrErr(t, src)
	if err == nil {
		t.Fatalf("%q => expected error containing %q", src, wantPart)
	}
	if !strings.Contains(err.Error(), wantPart) {
		t.Fatalf("%q => error %q should contain %q", src, err, wantPart)
	}
}

// ---- A: 求值顺序 ----

func TestLeftToRightEvaluationOrder(t *testing.T) {
	src := `var a=0
def f(x){
	a=a*10+x
	return x
}
var r=f(1)<f(2)
a`
	// 先算左边 f(1) 再算右边 f(2) => a = 12
	expect(t, src, "12")
}

func TestLessThanComparison(t *testing.T) {
	expect(t, "1<2", "true")
	expect(t, "2<1", "false")
	expect(t, "1<=1", "true")
	expect(t, "2<=1", "false")
	expect(t, "1.5<2", "true")
	expect(t, `"a"<"b"`, "true")
	expect(t, `"b"<="a"`, "false")
	expect(t, "1<2 and 3>2", "true")
}

// ---- B: break 作用域 ----

func TestBreakOnlyExitsInnermostLoop(t *testing.T) {
	src := `var i=0
var inner=0
for(;i<3;){
	i=i+1
	for(var j=0;j<5;j=j+1){
		inner=inner+1
		break
	}
}
inner`
	// 外层 3 轮，内层每轮只跑 1 次
	expect(t, src, "3")
}

func TestBreakInsideIfInsideLoop(t *testing.T) {
	src := `var i=0
for(;i<10;){
	i=i+1
	if(i==3){
		break
	}
}
i`
	expect(t, src, "3")
}

func TestBreakNestedLoopStillUsesOwnTarget(t *testing.T) {
	src := `var s=0
var i=0
for(;i<5;){
	i=i+1
	for(var j=0;j<3;j=j+1){
		s=s+1
		if(j==0){
			break
		}
	}
	if(i==2){
		break
	}
}
i`
	// 内层每轮 1 次 => s=2, 外层在 i==2 时 break => i=2
	expect(t, src, "2")
}

func TestBreakOutsideLoopIsCompileError(t *testing.T) {
	expectErr(t, "break\n1", "break outside of a loop")
}

func TestBreakInsideFunctionInsideLoopIsError(t *testing.T) {
	expectErr(t, `var i=0
for(;i<3;){
	i=i+1
}
def f(){
	break
}
f()`, "break outside of a loop")
}

// ---- C: 闭包访问外层局部变量 ----

func TestClosureOverOuterLocal(t *testing.T) {
	expect(t, `def outer(){
	var x=1
	def inner(){
		return x
	}
	return inner()
}
outer()`, "1")

	expect(t, `def outer(){
	var x=1
	def inner(){
		x=2
	}
	inner()
	return x
}
outer()`, "2")
}

func TestClosureOverOuterParam(t *testing.T) {
	expect(t, `def outer(n){
	def inner(){
		return n
	}
	return inner()
}
outer(1)`, "1")
}

// 顶层全局变量在函数里是可以读写的（这是现有的全局作用域语义）。
func TestFunctionCanUseGlobals(t *testing.T) {
	expect(t, `var g=10
def f(){
	g=g+5
	return g
}
f()`, "15")
}

// ---- D: 同名函数 ----

func TestDuplicateFunctionNamesResolveCorrectly(t *testing.T) {
	expect(t, `def f(){
	return 1
}
def f(){
	return 2
}
f()`, "2")
}

func TestFunctionInBlockDoesNotBreakOuter(t *testing.T) {
	expect(t, `def f(){
	return 1
}
if(true){
	def f(){
		return 2
	}
	len(f())
}
f()`, "1")
}

// ---- E: 内建函数返回值 ----

func TestPrintInExpressionIsNone(t *testing.T) {
	// print 没有返回值 => None，参与算术应给出清晰错误而不是破坏栈
	expectErr(t, `var x=print(1)+1`, "unsupported types")
}

func TestBuiltinWithoutResultAssignsNone(t *testing.T) {
	got, err := evalOrErr(t, `var a=print(1)
type(a)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "NULL") && !strings.Contains(got, "None") {
		t.Fatalf("print returns None, got type %q", got)
	}
}

// ---- F: 语句级表达式完整解析 ----

func TestStatementLevelCallKeepsInfix(t *testing.T) {
	// 语句级调用后面跟运算符不能被截断成两条语句
	expect(t, `def g(){
	return 1
}
g()+1`, "2")

	expect(t, `def g(){
	return 1
}
g()`, "1")
}
