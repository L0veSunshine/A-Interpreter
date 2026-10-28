package code

import "testing"

func TestMaxStackStraightLine(t *testing.T) {
	// 0: OpConstant 0   +1 -> 1
	// 3: OpConstant 1   +1 -> 2  (峰值 2)
	// 6: OpAdd          -1 -> 1
	// 7: OpPop          -1 -> 0
	prog := ins(
		op(OpConstant, 0),
		op(OpConstant, 1),
		op(OpAdd),
		op(OpPop),
	)
	got, err := MaxStack(prog)
	if err != nil {
		t.Fatalf("MaxStack: %v", err)
	}
	if got != 2 {
		t.Fatalf("max stack = %d, want 2", got)
	}
}

func TestMaxStackEmpty(t *testing.T) {
	if got, err := MaxStack(nil); err != nil || got != 0 {
		t.Fatalf("empty: got (%d, %v), want (0, nil)", got, err)
	}
}

// 两个分支各压一个值，汇合处深度必须一致，峰值是 1。
func TestMaxStackBranch(t *testing.T) {
	//  0: OpTrue            (1)  +1 -> 1
	//  1: OpJumpNotTrue 10  (3)  -1 -> 0   跳到 10（else 分支）
	//  4: OpConstant 0      (3)  +1 -> 1
	//  7: OpJump 11         (3)   0 -> 1   跳到 11（汇合点）
	// 10: OpNull            (1)  +1 -> 1
	// 11: OpPop             (1)  -1 -> 0
	prog := ins(
		op(OpTrue),
		op(OpJumpNotTrue, 10),
		op(OpConstant, 0),
		op(OpJump, 11),
		op(OpNull),
		op(OpPop),
	)
	got, err := MaxStack(prog)
	if err != nil {
		t.Fatalf("MaxStack: %v", err)
	}
	if got != 1 {
		t.Fatalf("max stack = %d, want 1", got)
	}
}

// 循环回边上的深度必须和循环入口一致，否则报错。
func TestMaxStackLoop(t *testing.T) {
	//  0: OpTrue            (1)  +1 -> 1
	//  1: OpJumpNotTrue 9   (3)  -1 -> 0   条件为假时退出到 9
	//  4: OpNull            (1)  +1 -> 1   循环体留一个值
	//  5: OpPop             (1)  -1 -> 0
	//  6: OpJump 0          (3)   0 -> 0   回边
	//  9: OpNull            (1)  +1 -> 1
	// 10: OpPop             (1)  -1 -> 0
	prog := ins(
		op(OpTrue),
		op(OpJumpNotTrue, 9),
		op(OpNull),
		op(OpPop),
		op(OpJump, 0),
		op(OpNull),
		op(OpPop),
	)
	got, err := MaxStack(prog)
	if err != nil {
		t.Fatalf("MaxStack: %v", err)
	}
	if got != 1 {
		t.Fatalf("max stack = %d, want 1", got)
	}
}

// 汇合点深度不一致说明字节码有问题，必须报错而不是给个错的上界。
func TestMaxStackRejectsInconsistentDepth(t *testing.T) {
	//  0: OpTrue            (1)  +1 -> 1
	//  1: OpJumpNotTrue 9   (3)  -1 -> 0   以深度 0 到达 9
	//  4: OpTrue            (1)  +1 -> 1
	//  5: OpNull            (1)  +1 -> 2
	//  6: OpJump 9          (3)   0 -> 2   以深度 2 到达 9  —— 冲突
	//  9: OpPop             (1)
	prog := ins(
		op(OpTrue),
		op(OpJumpNotTrue, 9),
		op(OpTrue),
		op(OpNull),
		op(OpJump, 9),
		op(OpPop),
	)
	if _, err := MaxStack(prog); err == nil {
		t.Fatal("expected an error for inconsistent stack depth")
	}
}

// 弹出多于压入（栈下溢）必须报错。
func TestMaxStackRejectsUnderflow(t *testing.T) {
	prog := ins(op(OpAdd))
	if _, err := MaxStack(prog); err == nil {
		t.Fatal("expected an error for stack underflow")
	}
}

// 跳到非指令边界必须报错。
func TestMaxStackRejectsBadJumpTarget(t *testing.T) {
	prog := ins(op(OpJump, 999))
	if _, err := MaxStack(prog); err == nil {
		t.Fatal("expected an error for out-of-range jump target")
	}
}

// 未知操作码必须报错（由调用方走保守兜底）。
func TestMaxStackRejectsUnknownOpcode(t *testing.T) {
	prog := ins(op(OpNull), []byte{0xfd})
	if _, err := MaxStack(prog); err == nil {
		t.Fatal("expected an error for unknown opcode")
	}
}

// 保守兜底永远成立：无法分析时用指令条数作为上界。
func TestConservativeMaxStackFallback(t *testing.T) {
	prog := ins(op(OpNull), []byte{0xfd}, op(OpPop))
	got := ConservativeMaxStack(prog)
	if got < 1 {
		t.Fatalf("conservative bound = %d, want at least 1", got)
	}
	if got > len(prog) {
		t.Fatalf("conservative bound = %d should not exceed instruction count %d", got, len(prog))
	}
}

// 可分析时 ConservativeMaxStack 必须给出精确值，而不是保守值。
func TestConservativeMaxStackExactWhenAnalyzable(t *testing.T) {
	prog := ins(op(OpConstant, 0), op(OpConstant, 1), op(OpAdd), op(OpPop))
	exact, err := MaxStack(prog)
	if err != nil {
		t.Fatalf("MaxStack: %v", err)
	}
	if got := ConservativeMaxStack(prog); got != exact {
		t.Fatalf("conservative = %d, want exact %d", got, exact)
	}
}

// 方法调用的栈效应不定（可能只返回一个值），按"压回两个"取上界仍是安全的，
// 因此必须能分析而不是报错。
func TestMaxStackHandlesCallMethod(t *testing.T) {
	prog := ins(
		op(OpGetLocal, 0),   // +1
		op(OpLoadMethod, 1), // +1 -> 2
		op(OpCallMethod, 0), // -0 -> 2（弹出 self+method，压回 self+ret）
		op(OpPop),           // -1 -> 1
		op(OpReturnVal),     // 终结
	)
	got, err := MaxStack(prog)
	if err != nil {
		t.Fatalf("MaxStack: %v", err)
	}
	if got != 2 {
		t.Fatalf("max stack = %d, want 2", got)
	}
}

// 融合指令的栈效应也要能算（+1），并且 OpReturnOperand 的峰值要算进去。
func TestMaxStackHandlesFusedOps(t *testing.T) {
	prog := ins(
		op(OpBinFused, int(OperandLocal<<4|OperandConst), int(OpAdd), 0, 0), // +1
		op(OpReturnOperand, int(OperandLocal), 0),                           // +1 -> 2（峰值）
	)
	got, err := MaxStack(prog)
	if err != nil {
		t.Fatalf("MaxStack: %v", err)
	}
	if got != 2 {
		t.Fatalf("max stack = %d, want 2", got)
	}
}
