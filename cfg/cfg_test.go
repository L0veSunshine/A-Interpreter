package cfg_test

import (
	// 用点导入让断言继续以 Build/Graph 的简写书写。
	// 本包必须是外部测试包：cfg 的测试需要 compiler，
	// 而 compiler 现在依赖 ir，ir 又依赖 cfg，放在包内会形成导入环。
	"Interpreter/bytecode"
	. "Interpreter/cfg"
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/object"
	"Interpreter/parser"
	"Interpreter/vm"
	"testing"
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
	c.SetOptimize(false)
	c.Compile(node)
	if c.HasError() {
		t.Fatalf("compile error: %v", c.Errs())
	}
	return c.ByteCode()
}

func runBytecode(b *bytecode.Bytecode) (string, error) {
	m := vm.NewVM()
	if err := m.Run(b); err != nil {
		return "", err
	}
	return m.LastPop().Inspect(), nil
}

// compactAll 对主函数和所有已编译函数的指令流分别做 CFG 压缩。
func compactAll(b *bytecode.Bytecode) (*bytecode.Bytecode, error) {
	out := &bytecode.Bytecode{
		Constants:  make([]object.Object, len(b.Constants)),
		Symbols:    b.Symbols,
		MainLocals: b.MainLocals,
	}
	for i, obj := range b.Constants {
		if fn, ok := obj.(object.CompiledFunc); ok {
			g, err := Build(fn.Instructions)
			if err != nil {
				return nil, err
			}
			ins, err := g.Compact(fn.Instructions)
			if err != nil {
				return nil, err
			}
			fn.Instructions = ins
			obj = fn
		}
		out.Constants[i] = obj
	}
	g, err := Build(b.Instruction)
	if err != nil {
		return nil, err
	}
	ins, err := g.Compact(b.Instruction)
	if err != nil {
		return nil, err
	}
	out.Instruction = ins
	return out, nil
}

func mustBuild(t *testing.T, b *bytecode.Bytecode) *Graph {
	t.Helper()
	g, err := Build(b.Instruction)
	if err != nil {
		t.Fatalf("build cfg: %v", err)
	}
	return g
}

func TestBuildFindsLoopsAndBranches(t *testing.T) {
	b := compileSrc(t, `var s=0
for(var i=0;i<10;i=i+1){
	if(i%2==0){
		s=s+i
	}
}
s`)
	g := mustBuild(t, b)
	if !g.HasBackEdge() {
		t.Fatal("for loop should produce a back edge")
	}
	condJumps := 0
	for _, blk := range g.Blocks {
		last, ok := blk.Last()
		if !ok {
			continue
		}
		if last.IsCondJump() {
			condJumps++
			if len(blk.Succ) != 2 {
				t.Fatalf("conditional jump block should have 2 successors, got %d", len(blk.Succ))
			}
		}
		if last.IsJump() && len(blk.Succ) != 1 {
			t.Fatalf("jump block should have 1 successor, got %d", len(blk.Succ))
		}
	}
	if condJumps < 2 {
		t.Fatalf("expected loop + if conditional jumps, got %d", condJumps)
	}
	if len(g.Unreachable()) != 0 {
		t.Fatalf("no dead code expected, got %d blocks", len(g.Unreachable()))
	}
}

func TestCompactDropsJumpToNextBlock(t *testing.T) {
	b := compileSrc(t, `var x=0
if (true){
	var y=1
	x=x+y
}
x`)
	before := mustBuild(t, b)
	jumpsBefore := 0
	for _, blk := range before.Blocks {
		if last, ok := blk.Last(); ok && last.IsJump() {
			jumpsBefore++
		}
	}
	if jumpsBefore == 0 {
		t.Fatal("if without else should emit an unnecessary OpJump")
	}

	out, err := before.Compact(b.Instruction)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	after, err := Build(out)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	for i, blk := range after.Blocks {
		last, ok := blk.Last()
		if !ok {
			continue
		}
		if last.IsJump() && i+1 < len(after.Blocks) && last.Operands[0] == after.Blocks[i+1].Start {
			t.Fatalf("block %d still jumps to the next block", blk.ID)
		}
	}
	if len(out) >= len(b.Instruction) {
		t.Fatalf("compaction should shrink instructions: %d -> %d", len(b.Instruction), len(out))
	}
}

func TestCompactDropsDeadCode(t *testing.T) {
	b := compileSrc(t, `def g(){
	return 7
	var dead=1
	dead=dead+1
}
g()`)
	var deadFound bool
	for _, obj := range b.Constants {
		fn, ok := obj.(object.CompiledFunc)
		if !ok {
			continue
		}
		g, err := Build(fn.Instructions)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if len(g.Unreachable()) == 0 {
			t.Fatalf("statements after return should be unreachable")
		}
		deadFound = true
		ins, err := g.Compact(fn.Instructions)
		if err != nil {
			t.Fatalf("compact: %v", err)
		}
		g2, err := Build(ins)
		if err != nil {
			t.Fatalf("rebuild: %v", err)
		}
		if len(g2.Unreachable()) != 0 {
			t.Fatalf("dead blocks should be gone after compact")
		}
	}
	if !deadFound {
		t.Fatal("expected a compiled function in the constant pool")
	}
}

func TestCompactKeepsSemantics(t *testing.T) {
	progs := []string{
		`var a=1
a=a+1
a`,
		`var x=0
if (true){
	var y=1
	x=x+y
}else{
	x=2
}
x`,
		`var s=0
for(var i=0;i<50;i=i+1){
	if(i%2==0){
		s=s+i
	}
}
s`,
		`var s=0
for(;s<100;){
	s=s+3
	if(s>50){
		break
	}
}
s`,
		`def f(n){
	if(n<=1){
		return 1
	}
	return n*f(n-1)
}
f(5)`,
		`def g(){
	return 7
}
var t=g()+1
t`,
	}
	for i, src := range progs {
		b := compileSrc(t, src)
		want, err := runBytecode(b)
		if err != nil {
			t.Fatalf("prog %d original run failed: %v", i, err)
		}
		opt, err := compactAll(b)
		if err != nil {
			t.Fatalf("prog %d compact failed: %v", i, err)
		}
		got, err := runBytecode(opt)
		if err != nil {
			t.Fatalf("prog %d optimized run failed: %v", i, err)
		}
		if got != want {
			t.Fatalf("prog %d result changed: want %s, got %s", i, want, got)
		}
		if len(opt.Instruction) > len(b.Instruction) {
			t.Fatalf("prog %d grew: %d -> %d", i, len(b.Instruction), len(opt.Instruction))
		}
	}
}
