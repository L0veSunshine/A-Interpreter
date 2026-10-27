package vm_test

import (
	"testing"

	"Interpreter/bytecode"
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/parser"
	"Interpreter/vm"
)

// compile 走完整前端（含 IR 优化），返回可直接执行的字节码。
func compile(b testing.TB, src string) *bytecode.Bytecode {
	b.Helper()
	p := parser.NewParser(lexer.NewLexer(src))
	node := p.Parse()
	if p.HasError() {
		b.Fatalf("parse error: %v", p.Errs())
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.Compile(node)
	if c.HasError() {
		b.Fatalf("compile error: %v", c.Errs())
	}
	return c.ByteCode()
}

// runOnce 执行一次，用来在基准里校验结果正确（避免优化成空循环）。
func runOnce(b testing.TB, bc *bytecode.Bytecode) string {
	b.Helper()
	m := vm.NewVM()
	if err := m.Run(bc); err != nil {
		b.Fatalf("run: %v", err)
	}
	return m.LastPop().Inspect()
}

// 纯整数运算 + 循环：反映"每指令派发开销"。
func BenchmarkLoopArith(b *testing.B) {
	bc := compile(b, `
var s=0
for(var i=0;i<1000;i=i+1){
	s=s+i*2-1
}
s`)
	if got := runOnce(b, bc); got != "998000" {
		b.Fatalf("unexpected result: %s", got)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := vm.NewVM()
		if err := m.Run(bc); err != nil {
			b.Fatal(err)
		}
	}
}

// 递归调用：反映"每调用帧的建帧开销"。
func BenchmarkFibRecursive(b *testing.B) {
	bc := compile(b, `
def fib(n){
	if(n<2){
		return n
	}
	return fib(n-1)+fib(n-2)
}
fib(18)`)
	if got := runOnce(b, bc); got != "2584" {
		b.Fatalf("unexpected result: %s", got)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := vm.NewVM()
		if err := m.Run(bc); err != nil {
			b.Fatal(err)
		}
	}
}

// 数组与下标：反映容器构造/下标访问的开销。
func BenchmarkArrayIndex(b *testing.B) {
	bc := compile(b, `
var a=[1,2,3,4,5,6,7,8]
var s=0
for(var i=0;i<200;i=i+1){
	s=s+a[i%8]
}
s`)
	if got := runOnce(b, bc); got != "900" {
		b.Fatalf("unexpected result: %s", got)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := vm.NewVM()
		if err := m.Run(bc); err != nil {
			b.Fatal(err)
		}
	}
}

// 深递归：反映帧栈扩容与局部槽位分配。
func BenchmarkDeepRecursion(b *testing.B) {
	bc := compile(b, `
def down(n){
	if(n<=0){
		return 0
	}
	return down(n-1)+1
}
down(200)`)
	if got := runOnce(b, bc); got != "200" {
		b.Fatalf("unexpected result: %s", got)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := vm.NewVM()
		if err := m.Run(bc); err != nil {
			b.Fatal(err)
		}
	}
}
