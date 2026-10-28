package code

import (
	"strings"
	"testing"
)

// 便捷构造：把若干指令拼成一段字节码。
func ins(bs ...[]byte) Instructions {
	var out Instructions
	for _, b := range bs {
		out = append(out, b...)
	}
	return out
}

func op(op Opcode, operands ...int) []byte { return Make(op, operands...) }

// decodeAll 是测试里用的解码小工具。
func decodeAll(t *testing.T, ins Instructions) []inst {
	t.Helper()
	insts, err := decode(ins)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return insts
}

func TestFuseOperandOperandBin(t *testing.T) {
	in := ins(
		op(OpGetLocal, 0),
		op(OpConstant, 3),
		op(OpAdd),
	)
	out, _, err := Fuse(in, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	insts := decodeAll(t, out)
	if len(insts) != 1 {
		t.Fatalf("want 1 instruction, got %d", len(insts))
	}
	if insts[0].op != OpBinFused {
		t.Fatalf("want OpBinFused, got %s", Definitions[insts[0].op].Name)
	}
	want := []int{int(OperandLocal<<4 | OperandConst), int(OpAdd), 0, 3}
	for i := range want {
		if insts[0].operands[i] != want[i] {
			t.Fatalf("operand %d = %d, want %d (all: %v)", i, insts[0].operands[i], want[i], insts[0].operands)
		}
	}
	// 融合后长度必须不增
	if len(out) > len(in) {
		t.Fatalf("fusion grew code: %d -> %d bytes", len(in), len(out))
	}
}

// 三种来源（局部/常量/全局）任意组合都能融合。
func TestFuseAcceptsAllOperandKinds(t *testing.T) {
	cases := []struct {
		name  string
		in    Instructions
		kinds byte
	}{
		{"local+local", ins(op(OpGetLocal, 1), op(OpGetLocal, 2), op(OpMul)),
			OperandLocal<<4 | OperandLocal},
		{"global+const", ins(op(OpGetGlobal, 1), op(OpConstant, 2), op(OpSub)),
			OperandGlobal<<4 | OperandConst},
		{"const+global", ins(op(OpConstant, 1), op(OpGetGlobal, 2), op(OpLT)),
			OperandConst<<4 | OperandGlobal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := Fuse(tc.in, nil)
			if err != nil {
				t.Fatalf("fuse: %v", err)
			}
			insts := decodeAll(t, out)
			if len(insts) != 1 || insts[0].op != OpBinFused {
				t.Fatalf("want one OpBinFused, got %v", insts)
			}
			if byte(insts[0].operands[0]) != tc.kinds {
				t.Fatalf("kindAB = %d, want %d", insts[0].operands[0], tc.kinds)
			}
		})
	}
}

func TestFuseOperandReturn(t *testing.T) {
	in := ins(op(OpGetLocal, 7), op(OpReturnVal))
	out, lines, err := Fuse(in, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	insts := decodeAll(t, out)
	if len(insts) != 1 || insts[0].op != OpReturnOperand {
		t.Fatalf("want one OpReturnOperand, got %v", insts)
	}
	if insts[0].operands[0] != int(OperandLocal) || insts[0].operands[1] != 7 {
		t.Fatalf("unexpected operands: %v", insts[0].operands)
	}
	if len(out) != len(in) {
		t.Fatalf("length should stay equal: %d -> %d", len(in), len(out))
	}
	_ = lines
}

func TestFuseBinReturn(t *testing.T) {
	in := ins(op(OpAdd), op(OpReturnVal))
	out, _, err := Fuse(in, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	insts := decodeAll(t, out)
	if len(insts) != 1 || insts[0].op != OpBinReturn {
		t.Fatalf("want one OpBinReturn, got %v", insts)
	}
	if insts[0].operands[0] != int(OpAdd) {
		t.Fatalf("want binOp OpAdd, got %d", insts[0].operands[0])
	}
	if len(out) != len(in) {
		t.Fatalf("length should stay equal: %d -> %d", len(in), len(out))
	}
}

// 跳转目标落在候选序列中间时绝不能融合，否则跳进去会错位。
func TestFuseRefusesWhenTargetInMiddle(t *testing.T) {
	// 0: OpJump 5    <- 跳到 OpConstant（候选序列的中间）
	// 3: OpGetLocal 0
	// 6: OpConstant 3
	// 9: OpAdd
	in := ins(
		op(OpJump, 6),
		op(OpGetLocal, 0),
		op(OpConstant, 3),
		op(OpAdd),
	)
	out, _, err := Fuse(in, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	for _, it := range decodeAll(t, out) {
		if it.op == OpBinFused {
			t.Fatal("must not fuse when a jump lands inside the sequence")
		}
	}
}

// 融合目标本身是跳转目标时是允许的（跳转指向融合指令的开头）。
func TestFuseAllowsTargetAtHead(t *testing.T) {
	in := ins(
		op(OpJump, 3),
		op(OpGetLocal, 0),
		op(OpConstant, 3),
		op(OpAdd),
	)
	out, _, err := Fuse(in, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	count := 0
	for _, it := range decodeAll(t, out) {
		if it.op == OpBinFused {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("want the sequence fused, got %d fused instructions", count)
	}
}

// 跨源码行的指令不能融合：否则运行时错误报的行号会变。
func TestFuseRefusesAcrossLines(t *testing.T) {
	in := ins(
		op(OpGetLocal, 0),
		op(OpConstant, 3),
		op(OpAdd),
	)
	lines := []LineEntry{{Offset: 0, Line: 1}, {Offset: 3, Line: 2}} // 第 2 条换行了
	out, _, err := Fuse(in, lines)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	for _, it := range decodeAll(t, out) {
		if it.op == OpBinFused {
			t.Fatal("must not fuse instructions from different source lines")
		}
	}
}

// 融合后跳转操作数要指向新的偏移，否则会跳到错误的位置。
func TestFuseRemapsJumpTargets(t *testing.T) {
	// 0: OpGetLocal 0        \
	// 3: OpConstant 1         > 会融合成一条（7 字节）
	// 6: OpAdd               /
	// 7: OpJumpNotTrue 13    <- 目标 13 = OpReturnVal
	// 10: OpNull
	// 11: OpPop
	// 12: OpReturnVal        <- 融合后应变成 5
	in := ins(
		op(OpGetLocal, 0),
		op(OpConstant, 1),
		op(OpAdd),
		op(OpJumpNotTrue, 13),
		op(OpNull),
		op(OpPop),
		op(OpReturnVal),
	)
	// 上面手算的偏移偏了一点，改用真实偏移：先解码确认
	insts := decodeAll(t, in)
	var retOff int
	for _, it := range insts {
		if it.op == OpReturnVal {
			retOff = it.offset
		}
	}
	// 重新构造一条指向 OpReturnVal 的跳转
	in = ins(
		op(OpGetLocal, 0),
		op(OpConstant, 1),
		op(OpAdd),
		op(OpJumpNotTrue, retOff),
		op(OpNull),
		op(OpPop),
		op(OpReturnVal),
	)
	out, _, err := Fuse(in, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	outInsts := decodeAll(t, out)
	var jump, ret *inst
	for i := range outInsts {
		if outInsts[i].op == OpJumpNotTrue {
			jump = &outInsts[i]
		}
		if outInsts[i].op == OpReturnVal {
			ret = &outInsts[i]
		}
	}
	if jump == nil || ret == nil {
		t.Fatalf("jump or return vanished: %v", outInsts)
	}
	if jump.operands[0] != ret.offset {
		t.Fatalf("jump target = %d, want remapped %d", jump.operands[0], ret.offset)
	}
}

// 行号表要跟着新偏移走，并且保持原来的行号。
func TestFuseKeepsLineNumbers(t *testing.T) {
	in := ins(
		op(OpGetLocal, 0),
		op(OpConstant, 1),
		op(OpAdd),
		op(OpPop),
	)
	lines := []LineEntry{{Offset: 0, Line: 5}, {Offset: 7, Line: 6}}
	out, outLines, err := Fuse(in, lines)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	// 融合后第一条指令（偏移 0）仍是第 5 行
	if got := LineAt(outLines, 0); got != 5 {
		t.Fatalf("line at 0 = %d, want 5", got)
	}
	// OpPop 的位置应该还是第 6 行（它是第二个源码行的开始）
	popOff := len(out) - 1
	if got := LineAt(outLines, popOff); got != 6 {
		t.Fatalf("line at %d = %d, want 6 (lines: %v)", popOff, got, outLines)
	}
}

// 未知操作码要原样返回 + 报错，调用方据此保留原指令流。
func TestFuseUnknownOpcodeReturnsOriginal(t *testing.T) {
	in := ins(op(OpNull), []byte{0xfe})
	out, lines, err := Fuse(in, nil)
	if err == nil {
		t.Fatal("expected an error for unknown opcode")
	}
	if len(out) != len(in) || &out[0] != &in[0] {
		t.Fatal("must return the original instruction stream")
	}
	if lines != nil {
		t.Fatal("lines should be returned unchanged (nil here)")
	}
	if !strings.Contains(err.Error(), "unknown opcode") {
		t.Fatalf("error should mention unknown opcode, got %v", err)
	}
}

// 没有可融合的地方时原样返回，不做多余分配。
func TestFuseNoopOnNonMatching(t *testing.T) {
	in := ins(op(OpTrue), op(OpPop), op(OpNull), op(OpReturnVal))
	out, _, err := Fuse(in, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("length changed: %d -> %d", len(in), len(out))
	}
}

// 融合后的指令流再融合一次必须不再变化（幂等）。
func TestFuseIsIdempotent(t *testing.T) {
	in := ins(
		op(OpGetLocal, 0),
		op(OpConstant, 1),
		op(OpAdd),
		op(OpReturnVal),
	)
	once, onceLines, err := Fuse(in, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	twice, _, err := Fuse(once, onceLines)
	if err != nil {
		t.Fatalf("second fuse: %v", err)
	}
	if len(twice) != len(once) {
		t.Fatalf("fusing twice changed the stream: %d -> %d", len(once), len(twice))
	}
}

// 融合绝不增加代码体积。
func TestFuseNeverGrowsCode(t *testing.T) {
	in := ins(
		op(OpGetLocal, 0), op(OpConstant, 1), op(OpAdd),
		op(OpGetLocal, 1), op(OpGetLocal, 2), op(OpMul),
		op(OpGetGlobal, 3), op(OpConstant, 4), op(OpSub),
		op(OpGetLocal, 5), op(OpReturnVal),
		op(OpAdd), op(OpReturnVal),
	)
	out, _, err := Fuse(in, nil)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	if len(out) > len(in) {
		t.Fatalf("fusion grew code: %d -> %d bytes", len(in), len(out))
	}
}

// 跳转目标越界时必须报错并保留原流，而不是产出坏字节码。
func TestFuseRejectsOutOfRangeTarget(t *testing.T) {
	in := ins(op(OpJump, 9999))
	if _, _, err := Fuse(in, nil); err == nil {
		t.Fatal("expected an error for an out-of-range jump target")
	}
}
