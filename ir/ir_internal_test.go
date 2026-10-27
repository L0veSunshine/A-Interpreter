package ir

import (
	"testing"

	"Interpreter/code"
	"Interpreter/object"
)

func testPool(objs ...object.Object) *constPool { return newConstPool(objs) }

func intPool(values ...int) *constPool {
	objs := make([]object.Object, len(values))
	for i, v := range values {
		objs[i] = &object.Int{Value: v}
	}
	return newConstPool(objs)
}

// code1 拼出"操作码 + 操作数"的字节码序列。
func code1(op code.Opcode, operands ...int) code.Instructions {
	return code.Make(op, operands...)
}

// 栈式字节码必须被提升成"每个值有唯一定义点"的三地址码。
func TestBuildPromotesStackToValues(t *testing.T) {
	var bc code.Instructions
	bc = append(bc, code1(code.OpConstant, 0)...)
	bc = append(bc, code1(code.OpConstant, 1)...)
	bc = append(bc, code1(code.OpAdd)...)
	bc = append(bc, code1(code.OpPop)...)

	fn, err := buildFunction("<t>", 0, bc, nil, intPool(1, 2), false)
	if err != nil {
		t.Fatalf("buildFunction: %v", err)
	}
	if fn == nil {
		t.Fatal("buildFunction declined unexpectedly")
	}
	if len(fn.Blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(fn.Blocks))
	}

	var ops []Op
	for _, in := range fn.Entry.Instrs {
		ops = append(ops, in.Op)
	}
	want := []Op{OpConst, OpConst, OpBin, OpSink, OpExit}
	if len(ops) != len(want) {
		t.Fatalf("want ops %v, got %v", want, ops)
	}
	for i := range want {
		if ops[i] != want[i] {
			t.Fatalf("op %d: want %v, got %v (all: %v)", i, want[i], ops[i], ops)
		}
	}

	bin := fn.Entry.Instrs[2]
	if len(bin.Args) != 2 || bin.Args[0] == bin.Args[1] {
		t.Fatalf("binary op should take two distinct operands, got %v", bin.Args)
	}
	// OpPop 必须保留成 OpSink：lastPop 是程序可观察的结果
	if fn.Entry.Instrs[3].Args[0] != bin.Dst {
		t.Fatal("OpSink should consume the add result")
	}
}

// 块边界上操作数栈非空时必须放弃优化（需要"栈上的 phi"，尚未建模）。
func TestBuildBailsWhenStackCrossesBlock(t *testing.T) {
	var bc code.Instructions
	bc = append(bc, code1(code.OpConstant, 0)...)    // 0: 留在栈上
	bc = append(bc, code1(code.OpConstant, 1)...)    // 3: 条件
	bc = append(bc, code1(code.OpJumpNotTrue, 9)...) // 6
	bc = append(bc, code1(code.OpPop)...)            // 8
	bc = append(bc, code1(code.OpPop)...)            // 9

	fn, err := buildFunction("<t>", 0, bc, nil, intPool(1, 2), false)
	if err == nil {
		t.Fatalf("expected the builder to decline, got function with %d blocks", len(fn.Blocks))
	}
}

// buildDiamond 构造 if/else 形状的 IR：
//
//	entry --branch--> then / else --> join
//
// 槽位 0 在两个分支里各写一次、在 join 里读一次，因此 SSA 必须插 phi。
func buildDiamond(t *testing.T) (*Function, *Block) {
	t.Helper()
	fn := &Function{Name: "<t>", pool: intPool(1, 2)}
	entry := &Block{ID: 0}
	then := &Block{ID: 1}
	els := &Block{ID: 2}
	join := &Block{ID: 3}
	fn.Blocks = []*Block{entry, then, els, join}
	fn.Entry = entry

	cond := fn.NewValue()
	fn.Emit(entry, &Instr{Op: OpConst, Dst: cond, Imm: 0})
	fn.Emit(entry, &Instr{Op: OpBranch, Args: []ValueID{cond}, Imm: 2, Imm2: 1})

	for _, b := range []*Block{then, els} {
		imm := 0
		if b == els {
			imm = 1
		}
		v := fn.NewValue()
		fn.Emit(b, &Instr{Op: OpConst, Dst: v, Imm: imm})
		fn.Emit(b, &Instr{Op: OpStoreLocal, Args: []ValueID{v}, Imm: 0})
		fn.Emit(b, &Instr{Op: OpJump, Imm: 3})
	}

	out := fn.NewValue()
	fn.Emit(join, &Instr{Op: OpLoadLocal, Dst: out, Imm: 0})
	fn.Emit(join, &Instr{Op: OpExit})

	fn.linkBlocks()
	return fn, join
}

func TestSSAInsertsPhiAtJoin(t *testing.T) {
	fn, join := buildDiamond(t)
	if err := fn.runSSA(); err != nil {
		t.Fatalf("runSSA: %v", err)
	}

	var phi *Instr
	for _, in := range join.Instrs {
		if in.Op == OpPhi {
			phi = in
			break
		}
	}
	if phi == nil {
		t.Fatalf("join block should contain a phi, got %v", join.Instrs)
	}
	if phi != join.Instrs[0] {
		t.Fatal("phi must be the first instruction of the block")
	}
	if len(phi.Args) != len(join.Preds) {
		t.Fatalf("phi should have one argument per predecessor: %d args, %d preds",
			len(phi.Args), len(join.Preds))
	}
	if phi.Args[0] == phi.Args[1] {
		t.Fatalf("phi arguments should differ, both are %d", phi.Args[0])
	}
	for _, b := range fn.Blocks {
		for _, in := range b.Instrs {
			if in.Op == OpStoreLocal || in.Op == OpLoadLocal {
				t.Fatalf("local access survived SSA in block %d", b.ID)
			}
		}
	}
}

// 线性代码里的局部变量完全不产生 phi。
func TestSSANoSingleBlockPhi(t *testing.T) {
	fn := &Function{Name: "<t>", pool: intPool(1)}
	entry := &Block{ID: 0}
	fn.Blocks = []*Block{entry}
	fn.Entry = entry

	v := fn.NewValue()
	fn.Emit(entry, &Instr{Op: OpConst, Dst: v, Imm: 0})
	fn.Emit(entry, &Instr{Op: OpStoreLocal, Args: []ValueID{v}, Imm: 0})
	out := fn.NewValue()
	fn.Emit(entry, &Instr{Op: OpLoadLocal, Dst: out, Imm: 0})
	fn.Emit(entry, &Instr{Op: OpSink, Args: []ValueID{out}})
	fn.linkBlocks()

	if err := fn.runSSA(); err != nil {
		t.Fatalf("runSSA: %v", err)
	}
	for _, in := range entry.Instrs {
		if in.Op == OpPhi {
			t.Fatal("straight-line code needs no phi")
		}
	}
}

// 同时活跃的值不能共用槽位，先死后才定义的值可以复用槽位。
func TestAllocatorSeparatesSimultaneouslyLiveValues(t *testing.T) {
	fn := &Function{Name: "<t>", pool: intPool(1)}
	entry := &Block{ID: 0}
	fn.Blocks = []*Block{entry}
	fn.Entry = entry

	a := fn.NewValue()
	b := fn.NewValue()
	fn.Emit(entry, &Instr{Op: OpMakeArray, Dst: a})
	fn.Emit(entry, &Instr{Op: OpMakeArray, Dst: b})
	sum := fn.NewValue()
	fn.Emit(entry, &Instr{Op: OpBin, Dst: sum, Args: []ValueID{a, b}, Imm: int(code.OpAdd)})
	fn.Emit(entry, &Instr{Op: OpSink, Args: []ValueID{sum}})
	// a、b 到这里都已经死了，later 可以复用它们占用的槽位
	later := fn.NewValue()
	fn.Emit(entry, &Instr{Op: OpMakeArray, Dst: later})
	fn.Emit(entry, &Instr{Op: OpSink, Args: []ValueID{later}})
	fn.Emit(entry, &Instr{Op: OpExit})
	fn.linkBlocks()

	if err := fn.runSSA(); err != nil {
		t.Fatalf("runSSA: %v", err)
	}
	fn.allocateSlots()
	if fn.Slot(a) < 0 || fn.Slot(b) < 0 {
		t.Fatalf("live values must get slots, got %d and %d", fn.Slot(a), fn.Slot(b))
	}
	if fn.Slot(a) == fn.Slot(b) {
		t.Fatalf("values alive at the same time share slot %d", fn.Slot(a))
	}
	if s := fn.Slot(later); s != fn.Slot(a) && s != fn.Slot(b) {
		t.Fatalf("dead value's slot should be reused, later got %d, a=%d b=%d",
			s, fn.Slot(a), fn.Slot(b))
	}
}

// DCE 只能删除"既无副作用又不会报错"的指令。
func TestDeadCodeKeepsTrappingOps(t *testing.T) {
	cases := []struct {
		name    string
		op      Op
		imm     int
		nargs   int
		survive bool
	}{
		{"const", OpConst, 0, 0, false},
		{"make-array", OpMakeArray, 0, 0, false},
		{"add", OpBin, int(code.OpAdd), 2, true},
		{"div", OpBin, int(code.OpDiv), 2, true},
		{"index", OpIndex, 0, 2, true},
		{"unary-minus", OpUn, int(code.OpMinus), 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn := &Function{Name: "<t>", pool: intPool(1, 2)}
			entry := &Block{ID: 0}
			fn.Blocks = []*Block{entry}
			fn.Entry = entry

			in := &Instr{Op: tc.op, Dst: fn.NewValue(), Imm: tc.imm}
			for i := 0; i < tc.nargs; i++ {
				in.Args = append(in.Args, fn.NewValue())
			}
			fn.Emit(entry, in)
			fn.Emit(entry, &Instr{Op: OpExit})
			fn.linkBlocks()

			fn.removeDeadCode()

			found := false
			for _, x := range entry.Instrs {
				if x == in {
					found = true
				}
			}
			if found != tc.survive {
				t.Fatalf("survive=%v, but instruction %v", tc.survive, found)
			}
		})
	}
}

func TestFoldBinaryRefusesToFoldErrors(t *testing.T) {
	cases := []struct {
		name string
		op   code.Opcode
		x, y object.Object
	}{
		{"div0", code.OpDiv, &object.Int{Value: 1}, &object.Int{Value: 0}},
		{"mod0", code.OpMod, &object.Int{Value: 1}, &object.Int{Value: 0}},
		{"string+int", code.OpAdd, &object.String{Value: []rune("a")}, &object.Int{Value: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := foldBinary(tc.op, tc.x, tc.y); ok {
				t.Fatalf("%s must not be folded: it raises a runtime error",
					code.Definitions[tc.op].Name)
			}
		})
	}
}
