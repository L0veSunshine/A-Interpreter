package ir_test

import (
	"strings"
	"testing"

	"Interpreter/bytecode"
	"Interpreter/cfg"
	"Interpreter/code"
	"Interpreter/compiler"
	"Interpreter/ir"
	"Interpreter/lexer"
	"Interpreter/object"
	"Interpreter/parser"
	"Interpreter/vm"
)

// compileOptimized 走完整前端并开启 IR 优化；fusion 控制是否做超指令融合。
func compileOptimized(t *testing.T, src string, fusion bool) *bytecode.Bytecode {
	t.Helper()
	old := ir.FusionEnabled
	ir.FusionEnabled = fusion
	defer func() { ir.FusionEnabled = old }()

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

// scan 遍历一段指令流，统计指令条数与融合指令条数。
func scan(ins code.Instructions) (total, fused int) {
	for i := 0; i < len(ins); {
		op := code.Opcode(ins[i])
		def, ok := code.Definitions[op]
		if !ok {
			return total, fused
		}
		width := 1
		for _, w := range def.OperandWidth {
			width += w
		}
		switch op {
		case code.OpBinFused, code.OpReturnOperand, code.OpBinReturn:
			fused++
		}
		total++
		i += width
	}
	return total, fused
}

// scanAll 统计 main + 所有编译后函数的指令条数与融合条数。
func scanAll(bc *bytecode.Bytecode) (total, fused int) {
	t, f := scan(bc.Instruction)
	total, fused = total+t, fused+f
	for _, obj := range bc.Constants {
		if fn, ok := obj.(object.CompiledFunc); ok {
			t2, f2 := scan(fn.Instructions)
			total, fused = total+t2, fused+f2
		}
	}
	return total, fused
}

// 融合开/关必须**逐字等价**：返回值相同、错误信息相同（含源码行）。
func TestFusionPreservesBehavior(t *testing.T) {
	for _, tc := range corpus {
		t.Run(tc.name, func(t *testing.T) {
			fused := compileOptimized(t, tc.src, true)
			plain := compileOptimized(t, tc.src, false)

			got, gotErr := runBC(fused)
			want, wantErr := runBC(plain)

			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("error behavior changed: plain=%v fused=%v", wantErr, gotErr)
			}
			if wantErr != nil {
				if wantErr.Error() != gotErr.Error() {
					t.Fatalf("error message changed:\n plain=%v\n fused=%v", wantErr, gotErr)
				}
				return
			}
			if got != want {
				t.Fatalf("result changed: plain=%s fused=%s", want, got)
			}
		})
	}
}

// 已知含固定搭配的程序必须真的被融合，否则说明匹配规则失效了。
func TestFusionActuallyFires(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		minFused int
	}{
		{"recursion", `def fib(n){
	if(n<2){
		return n
	}
	return fib(n-1)+fib(n-2)
}
fib(10)`, 3},
		{"loop", `var s=0
for(var i=0;i<10;i=i+1){
	s=s+i
}
s`, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bc := compileOptimized(t, tc.src, true)
			_, fused := scanAll(bc)
			if fused < tc.minFused {
				t.Fatalf("expected at least %d fused instructions, got %d", tc.minFused, fused)
			}
		})
	}
}

// 融合后指令条数必须减少（派发次数下降），字节数不增加。
func TestFusionReducesInstructionCount(t *testing.T) {
	const src = `var s=0
for(var i=0;i<20;i=i+1){
	s=s+i*2
}
s`
	fused := compileOptimized(t, src, true)
	plain := compileOptimized(t, src, false)

	nFused, _ := scanAll(fused)
	nPlain, _ := scanAll(plain)
	if nFused >= nPlain {
		t.Fatalf("instruction count did not drop: %d -> %d", nPlain, nFused)
	}
	if len(fused.Instruction) > len(plain.Instruction) {
		t.Fatalf("code grew: %d -> %d bytes", len(plain.Instruction), len(fused.Instruction))
	}
}

// 融合指令里发生运行时错误时，行号必须和未融合时一致。
func TestFusionKeepsErrorLine(t *testing.T) {
	cases := []string{
		"var a=1\nvar b=0\na/b",
		"var a=1\nvar b=0\nvar c=a%b",
		"var s=\"x\"\ns-1",
	}
	for _, src := range cases {
		fused := compileOptimized(t, src, true)
		plain := compileOptimized(t, src, false)
		_, wantErr := runBC(plain)
		_, gotErr := runBC(fused)
		if wantErr == nil || gotErr == nil {
			t.Fatalf("%q: expected runtime errors, got plain=%v fused=%v", src, wantErr, gotErr)
		}
		// 错误信息里必须带源码行（否则说明行号表被融合弄丢了）
		if !strings.Contains(wantErr.Error(), "line ") {
			t.Fatalf("%q: baseline error should carry a source line, got %v", src, wantErr)
		}
		if gotErr.Error() != wantErr.Error() {
			t.Fatalf("%q: error changed with fusion:\n plain=%v\n fused=%v", src, wantErr, gotErr)
		}
	}
}

// 融合后的字节码必须仍能通过 CFG 校验（跳转目标都在范围内、指令可解码）。
func TestFusedBytecodeWellFormed(t *testing.T) {
	for _, tc := range corpus {
		bc := compileOptimized(t, tc.src, true)
		if _, err := cfg.Build(bc.Instruction); err != nil {
			t.Fatalf("%s: main instructions malformed: %v", tc.name, err)
		}
		for _, obj := range bc.Constants {
			fn, ok := obj.(object.CompiledFunc)
			if !ok {
				continue
			}
			if _, err := cfg.Build(fn.Instructions); err != nil {
				t.Fatalf("%s: function %s malformed: %v", tc.name, fn.FnName, err)
			}
		}
	}
}

// 关闭融合后不应出现任何融合指令。
func TestFusionDisabledEmitsNoFusedOps(t *testing.T) {
	const src = "var a=1\nvar b=2\na+b"
	bc := compileOptimized(t, src, false)
	if _, fused := scanAll(bc); fused != 0 {
		t.Fatalf("expected no fused instructions, got %d", fused)
	}
}

// 同一个 VM 连续跑融合/未融合两种版本，结果必须一致（防止状态串扰）。
func TestFusionRunsBackToBack(t *testing.T) {
	const src = `def inc(x){
	return x+1
}
var t=0
for(var i=0;i<5;i=i+1){
	t=inc(t)
}
t`
	fused := compileOptimized(t, src, true)
	plain := compileOptimized(t, src, false)
	for i := 0; i < 3; i++ {
		if got, _ := runBC(fused); got != "5" {
			t.Fatalf("fused run %d = %s, want 5", i, got)
		}
		if got, _ := runBC(plain); got != "5" {
			t.Fatalf("plain run %d = %s, want 5", i, got)
		}
	}
}

// 融合指令不能导致操作数栈累积（每轮循环泄漏一个槽位会在这里暴露）。
func TestFusionDoesNotLeakStack(t *testing.T) {
	const src = `var s=0
for(var i=0;i<50;i=i+1){
	s=s+i
}
s`
	fused := compileOptimized(t, src, true)
	m := vm.NewVM()
	if err := m.Run(fused); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := m.LastPop().Inspect(); got != "1225" {
		t.Fatalf("result = %s, want 1225", got)
	}
	if depth := m.MaxStackDepth(); depth > 16 {
		t.Fatalf("operand stack grew to %d slots, expected a small constant peak", depth)
	}
}

// 融合指令必须能正确处理任意操作数来源（局部 / 常量 / 全局）。
func TestFusionOperandSources(t *testing.T) {
	const src = `var g=10
def f(x){
	var y=x+g
	return y*2
}
f(3)`
	bc := compileOptimized(t, src, true)
	got, err := runBC(bc)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got != "26" {
		t.Fatalf("result = %s, want 26", got)
	}
}

// 静态栈上界必须覆盖所有语料。
//
// VM 现在不再逐条指令检查操作数栈容量（改由建帧时按 code.MaxStack 预留），
// 所以一旦上界算小就会越界。这里对全部语料跑一遍：任何
// "internal: operand stack overflow" 都说明上界算错了。
func TestStaticStackBoundCoversCorpus(t *testing.T) {
	for _, tc := range corpus {
		bc := compileOptimized(t, tc.src, true)
		m := vm.NewVM()
		err := m.Run(bc)
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), "internal") {
			t.Fatalf("%s: static stack bound is too small: %v", tc.name, err)
		}
	}
}
