package compiler

import (
	"Interpreter/code"
	"Interpreter/lexer"
	"Interpreter/parser"
	"testing"
)

// compileSrc 走完整前端；optimize 控制是否开启 IR 优化。
func compileSrc(t *testing.T, src string, optimize bool) *Compiler {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(src))
	node := p.Parse()
	if p.HasError() {
		t.Fatalf("parse error: %v", p.Errs())
	}
	c := NewCompiler()
	c.SetSymbol(p.SymTable)
	c.SetOptimize(optimize)
	c.Compile(node)
	if c.HasError() {
		t.Fatalf("compile error: %v", c.Errs())
	}
	return c
}

// 优化默认开启，并且确实让字节码变小。
func TestOptimizationIsOnByDefault(t *testing.T) {
	cases := []string{
		"1+2*3",
		"var a=1\na=a+1\na",
		"var a=0\nif(2*3>5){\n\ta=1\n}else{\n\ta=2\n}\na",
	}
	for _, src := range cases {
		on := compileSrc(t, src, true)
		off := compileSrc(t, src, false)
		if !on.optimize {
			t.Fatal("optimizer should be enabled by default")
		}
		got, base := on.ByteCode(), off.ByteCode()
		if len(got.Instruction) >= len(base.Instruction) {
			t.Fatalf("%q: optimized code is not smaller: %d -> %d",
				src, len(base.Instruction), len(got.Instruction))
		}
		if got.MainLocals < 0 {
			t.Fatalf("%q: MainLocals must not be negative", src)
		}
	}
}

// 关掉优化时必须原样返回编译器直出的字节码（包括 MainLocals）。
func TestOptimizationCanBeDisabled(t *testing.T) {
	const src = "var a=1\na=a+1\na"
	off := compileSrc(t, src, false)
	if off.result != nil {
		t.Fatal("optimized result should be nil when the optimizer is off")
	}
	bc := off.ByteCode()
	ct := off.symTable
	for ct.Outer != nil {
		ct = ct.Outer
	}
	if bc.MainLocals != ct.LocalSlots() {
		t.Fatalf("MainLocals = %d, want %d", bc.MainLocals, ct.LocalSlots())
	}
	if len(bc.Instruction) == 0 {
		t.Fatal("bytecode should not be empty")
	}
}

// 优化器放弃时（例如 REPL 模式的隐式打印），编译器必须回退到未优化的字节码，
// 而不是丢掉程序。
func TestOptimizeFallsBackWhenUnsupported(t *testing.T) {
	p := parser.NewParser(lexer.NewLexer("1+2"))
	node := p.Parse()
	if p.HasError() {
		t.Fatalf("parse error: %v", p.Errs())
	}
	c := NewCompiler()
	c.SetSymbol(p.SymTable)
	c.SetMode() // interpreter 模式会追加 OpPrintTop，IR 尚未建模
	c.Compile(node)
	if c.HasError() {
		t.Fatalf("compile error: %v", c.Errs())
	}
	bc := c.ByteCode()
	if len(bc.Instruction) == 0 {
		t.Fatal("bytecode should not be empty after falling back")
	}
	last := bc.Instruction[len(bc.Instruction)-1]
	if code.Opcode(last) != code.OpPrintTop {
		t.Fatalf("last opcode = %d, want OpPrintTop", last)
	}
}
