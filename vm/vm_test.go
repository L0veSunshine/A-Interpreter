package vm_test

import (
	"testing"

	"Interpreter/bytecode"
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/parser"
	"Interpreter/vm"
)

func compileSrc(t *testing.T, src string) *bytecode.Bytecode {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(src))
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
	return c.ByteCode()
}

func runSrc(t *testing.T, src string) (*vm.VM, string) {
	t.Helper()
	m := vm.NewVM()
	if err := m.Run(compileSrc(t, src)); err != nil {
		t.Fatalf("run: %v", err)
	}
	return m, m.LastPop().Inspect()
}

// 调用不该泄漏共享的局部槽位：返回后 localsTop 必须回到调用前的高度。
func TestCallsDoNotLeakLocalSlots(t *testing.T) {
	src := `
def f(a){
	var b=a+1
	return b
}
var s=0
for(var i=0;i<50;i=i+1){
	s=s+f(i)
}
s`
	m := vm.NewVM()
	bc := compileSrc(t, src)
	if err := m.Run(bc); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := m.LastPop().Inspect(); got != "1275" {
		t.Fatalf("result = %s, want 1275", got)
	}
	// main 自己的槽位仍占着，但 f 的槽位全部已归还
	if mainLocals := bc.MainLocals; m.LocalsDepth() != mainLocals {
		t.Fatalf("locals leaked: depth = %d, want %d (main only)", m.LocalsDepth(), mainLocals)
	}
}

// 深递归既要正确，也要保证栈与帧按需增长、返回后槽位全部归还。
func TestDeepRecursionReturnsAllSlots(t *testing.T) {
	src := `
def down(n){
	if(n<=0){
		return 0
	}
	return down(n-1)+1
}
down(300)`
	m := vm.NewVM()
	bc := compileSrc(t, src)
	if err := m.Run(bc); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := m.LastPop().Inspect(); got != "300" {
		t.Fatalf("result = %s, want 300", got)
	}
	if mainLocals := bc.MainLocals; m.LocalsDepth() != mainLocals {
		t.Fatalf("locals leaked: depth = %d, want %d", m.LocalsDepth(), mainLocals)
	}
	if m.MaxStackDepth() <= 0 {
		t.Fatal("MaxStackDepth should record the peak")
	}
}

// 局部变量在块作用域里声明、跨调用复用同一批槽位，不能串值。
func TestRepeatedCallsDoNotShareLocals(t *testing.T) {
	src := `
def f(n){
	var a=n*10
	if(a>0){
		var b=a+1
		a=b
	}
	return a
}
f(1)+f(2)*100`
	if _, got := runSrc(t, src); got != "2111" {
		t.Fatalf("result = %s, want 2111", got)
	}
}

// 内建函数/方法调用不改变槽位区高度。
func TestBuiltinCallsKeepLocalsDepth(t *testing.T) {
	src := `
var a=[3,1,2]
var n=len(a)
n`
	m, got := runSrc(t, src)
	if got != "3" {
		t.Fatalf("result = %s, want 3", got)
	}
	if m.LocalsDepth() <= 0 {
		t.Fatal("main frame should still own its slots")
	}
}

// 同一个 VM 在报错中断后还能继续跑下一个程序（REPL 场景）。
func TestVMCanBeReusedAfterError(t *testing.T) {
	m := vm.NewVM()
	bad := compileSrc(t, "def f(n){\n\treturn n/0\n}\nf(1)")
	if err := m.Run(bad); err == nil {
		t.Fatal("expected a runtime error")
	}
	good := compileSrc(t, "var a=1\na+2")
	if err := m.Run(good); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if got := m.LastPop().Inspect(); got != "3" {
		t.Fatalf("result = %s, want 3", got)
	}
}
