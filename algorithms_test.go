package main

import (
	"os"
	"path/filepath"
	"testing"

	"Interpreter/bytecode"
	"Interpreter/cfg"
	"Interpreter/code"
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/object"
	"Interpreter/parser"
	"Interpreter/vm"
)

// bench/algorithms 下的每个 .xl 都有一份 .py 配对实现（同一算法、同一输入、
// 同一输出），性能对比由 bench/compare.py 完成。这里负责：
//   - 校验 xlang 侧结果；
//   - 校验"优化开/关"结果一致（优化器契约）；
//   - 提供进程内基准（不含启动开销）。
var algorithmPrograms = []struct {
	name string
	want string
}{
	{"fib", "832040"},
	{"quicksort", "[1, 1493711048]"},
	{"mergesort", "[1, 1502504744]"},
	{"sieve", "78498"},
	{"nqueens", "2680"},
	{"lcs", "127"},
	{"knapsack", "14304"},
	{"matmul", "34996488"},
	{"two_sum", "197776"},
	{"binary_search", "[133959, 59943324736]"},
	{"collatz", "[35655, 324]"},
	{"heapsort", "[1, 10020395936]"},
	{"bfs_grid", "[798, 119342]"},
}

func algorithmSource(tb testing.TB, name string) string {
	tb.Helper()
	path := filepath.Join("bench", "algorithms", name+".xl")
	src, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read %s: %v", path, err)
	}
	return string(src)
}

func compileAlgorithm(tb testing.TB, src string, optimize bool) *bytecode.Bytecode {
	tb.Helper()
	p := parser.NewParser(lexer.NewLexer(src))
	node := p.Parse()
	if p.HasError() {
		tb.Fatalf("parse error: %v", p.Errs())
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.SetOptimize(optimize)
	c.Compile(node)
	if c.HasError() {
		tb.Fatalf("compile error: %v", c.Errs())
	}
	return c.ByteCode()
}

func runAlgorithm(tb testing.TB, bc *bytecode.Bytecode) string {
	tb.Helper()
	m := vm.NewVM()
	if err := m.Run(bc); err != nil {
		tb.Fatalf("run error: %v", err)
	}
	return m.LastPop().Inspect()
}

func TestAlgorithmCorpus(t *testing.T) {
	for _, prog := range algorithmPrograms {
		t.Run(prog.name, func(t *testing.T) {
			src := algorithmSource(t, prog.name)
			for _, optimize := range []bool{true, false} {
				got := runAlgorithm(t, compileAlgorithm(t, src, optimize))
				if got != prog.want {
					t.Fatalf("optimize=%v: want %s, got %s", optimize, prog.want, got)
				}
			}
		})
	}
}

// 每个算法在 benchmark 里的成绩就是"纯计算"时间（编译一次，反复执行）。
func BenchmarkAlgorithms(b *testing.B) {
	for _, prog := range algorithmPrograms {
		b.Run(prog.name, func(b *testing.B) {
			bc := compileAlgorithm(b, algorithmSource(b, prog.name), true)
			if got := runAlgorithm(b, bc); got != prog.want {
				b.Fatalf("want %s, got %s", prog.want, got)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m := vm.NewVM()
				if err := m.Run(bc); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

const appendLoopSource = `def collect(n){
	var a=[]
	for(var i=0;i<n;i=i+1){
		a.append(i)
	}
	return len(a)
}
collect(20000)`

func TestAppendLoopUsesOptimizedMethodPath(t *testing.T) {
	raw := compileAlgorithm(t, appendLoopSource, false)
	optimized := compileAlgorithm(t, appendLoopSource, true)
	for _, constant := range optimized.Constants {
		fn, ok := constant.(object.CompiledFunc)
		if !ok || fn.FnName != "collect" {
			continue
		}
		ops, err := cfg.Decode(fn.Instructions)
		if err != nil {
			t.Fatalf("decode optimized collect: %v", err)
		}
		fusedBranch, fusedBinary, methodCall := false, false, false
		for _, op := range ops {
			switch op.Opcode {
			case code.OpCmpBranch:
				fusedBranch = true
			case code.OpBinFused:
				fusedBinary = true
			case code.OpCallMethod:
				methodCall = true
			}
		}
		if !fusedBranch || !fusedBinary || !methodCall {
			t.Fatalf("optimized collect lacks fused loop operations or method call: branch=%v binary=%v method=%v",
				fusedBranch, fusedBinary, methodCall)
		}
		if got, want := runAlgorithm(t, optimized), runAlgorithm(t, raw); got != want {
			t.Fatalf("optimized result changed: raw=%s optimized=%s", want, got)
		}
		return
	}
	t.Fatal("collect function not found in optimized constants")
}

// BenchmarkAppendLoopOptimization compares the same append-heavy function with
// IR optimization enabled and disabled. Compilation is outside the timed loop.
func BenchmarkAppendLoopOptimization(b *testing.B) {
	for _, optimize := range []bool{false, true} {
		name := "disabled"
		if optimize {
			name = "enabled"
		}
		b.Run(name, func(b *testing.B) {
			bc := compileAlgorithm(b, appendLoopSource, optimize)
			if got := runAlgorithm(b, bc); got != "20000" {
				b.Fatalf("want 20000, got %s", got)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m := vm.NewVM()
				if err := m.Run(bc); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
