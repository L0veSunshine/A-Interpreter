package code

import "testing"

// 比较 + 条件跳转要融成一条 OpCmpBranch，且内嵌的跳转目标要能重映射。
func TestFuseCmpBranch(t *testing.T) {
	//  0: OpGetLocal 0        (3)
	//  3: OpConstant 1        (3)
	//  6: OpLT                (1)
	//  7: OpJumpNotTrue 10    (3)  -> 目标 10
	// 10: OpNull              (1)
	prog := ins(
		op(OpGetLocal, 0),
		op(OpConstant, 1),
		op(OpLT),
		op(OpJumpNotTrue, 10),
		op(OpNull),
	)
	out, _, err := Fuse(prog, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	insts := decodeAll(t, out)
	if len(insts) != 2 {
		t.Fatalf("want 2 instructions, got %d: %v", len(insts), insts)
	}
	fused := insts[0]
	if fused.op != OpCmpBranch {
		t.Fatalf("want OpCmpBranch, got %s", Definitions[fused.op].Name)
	}
	want := []int{int(OperandLocal<<4 | OperandConst), int(OpLT), 0, 1}
	for i := range want {
		if fused.operands[i] != want[i] {
			t.Fatalf("operand %d = %d, want %d (all: %v)", i, fused.operands[i], want[i], fused.operands)
		}
	}
	// 目标必须重映射到 OpNull 的新位置
	if got := fused.operands[4]; got != insts[1].offset {
		t.Fatalf("branch target = %d, want %d (the OpNull offset)", got, insts[1].offset)
	}
	// 融合后不增加字节数
	if len(out) > len(prog) {
		t.Fatalf("fusion grew code: %d -> %d bytes", len(prog), len(out))
	}
}

// 有跳转落到"比较"这条指令上时不能融合（否则跳进去会跳过比较）。
func TestFuseCmpBranchRefusesWhenTargetInMiddle(t *testing.T) {
	//  0: OpJump 6          跳到 OpLT
	//  3: OpGetLocal 0
	//  6: OpConstant 1
	//  9: OpLT
	// 10: OpJumpNotTrue 13
	// 13: OpNull
	prog := ins(
		op(OpJump, 9),
		op(OpGetLocal, 0),
		op(OpConstant, 1),
		op(OpLT),
		op(OpJumpNotTrue, 13),
		op(OpNull),
	)
	out, _, err := Fuse(prog, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	for _, it := range decodeAll(t, out) {
		if it.op == OpCmpBranch {
			t.Fatal("must not fuse when a jump lands inside the sequence")
		}
	}
}

// 比较与跳转不在同一源码行时不能融合（否则报错行号会变）。
func TestFuseCmpBranchRefusesAcrossLines(t *testing.T) {
	prog := ins(
		op(OpGetLocal, 0),
		op(OpConstant, 1),
		op(OpLT),
		op(OpJumpNotTrue, 10),
		op(OpNull),
	)
	lines := []LineEntry{{Offset: 0, Line: 1}, {Offset: 7, Line: 2}}
	out, _, err := Fuse(prog, lines)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	for _, it := range decodeAll(t, out) {
		if it.op == OpCmpBranch {
			t.Fatal("must not fuse instructions from different source lines")
		}
	}
}

// 算术运算不能被 FusecmpBranch 规则接管（它只认比较）。
func TestFuseCmpBranchIgnoresArithmetic(t *testing.T) {
	prog := ins(
		op(OpGetLocal, 0),
		op(OpConstant, 1),
		op(OpAdd),
		op(OpJumpNotTrue, 10),
		op(OpNull),
	)
	out, _, err := Fuse(prog, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	insts := decodeAll(t, out)
	if insts[0].op != OpBinFused {
		t.Fatalf("arithmetic should fuse into OpBinFused, got %s", Definitions[insts[0].op].Name)
	}
	for _, it := range insts {
		if it.op == OpCmpBranch {
			t.Fatal("arithmetic must not become OpCmpBranch")
		}
	}
}

// 两条新指令的栈效应必须能算出来（否则 MaxStack 会退化成保守上界）。
func TestStackDeltaCoversNewOpcodes(t *testing.T) {
	cases := []struct {
		op   Opcode
		args []int
		want int
	}{
		{OpCmpBranch, []int{0, int(OpLT), 0, 0, 0}, 0},
		{OpCallFuncConst, []int{0, 0}, 1},
		{OpCallFuncConst, []int{0, 1}, 0},
		{OpCallFuncConst, []int{0, 2}, -1},
	}
	for _, tc := range cases {
		got, err := stackDelta(tc.op, tc.args)
		if err != nil {
			t.Fatalf("%s: %v", Definitions[tc.op].Name, err)
		}
		if got != tc.want {
			t.Fatalf("%s%v delta = %d, want %d", Definitions[tc.op].Name, tc.args, got, tc.want)
		}
	}
}

// MaxStack 必须把 OpCmpBranch 当成"有两个后继的条件分支"来分析，
// 否则条件之后的代码永远不会被访问到，上界会被算小。
func TestMaxStackFollowsCmpBranch(t *testing.T) {
	//  0: OpCmpBranch -> 假跳 11   (9 字节，栈深 0 进 0 出)
	//  9: OpNull                   -> 1
	// 10: OpPop                    -> 0   （顺序落到 11）
	// 11: OpNull                   -> 1   （假分支目标）
	// 12: OpNull                   -> 2   ← 峰值
	// 13: OpPop                    -> 1
	// 14: OpPop                    -> 0
	prog := ins(
		Make(OpCmpBranch, 0, int(OpLT), 0, 0, 11),
		op(OpNull),
		op(OpPop),
		op(OpNull),
		op(OpNull),
		op(OpPop),
		op(OpPop),
	)
	got, err := MaxStack(prog)
	if err != nil {
		t.Fatalf("MaxStack: %v", err)
	}
	if got != 2 {
		t.Fatalf("max stack = %d, want 2 (false branch pushes twice)", got)
	}

	// 如果分析漏掉假分支，峰值只会算到 1——这正是"上界算小"的失败模式
	// 用一个不会走到假分支的取值来对照：把目标指到末尾之后也不会崩
	prog2 := ins(
		Make(OpCmpBranch, 0, int(OpLT), 0, 0, 9),
		op(OpNull),
		op(OpPop),
	)
	if _, err := MaxStack(prog2); err != nil {
		t.Fatalf("MaxStack(prog2): %v", err)
	}
}
