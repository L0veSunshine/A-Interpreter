package ir

import (
	"testing"

	"Interpreter/object"
)

// 构造一个"常量条件 if/else"形状的 IR：
//
//	entry --branch--> then / else --> join
//
// 两个分支都写本地槽位 0，join 里读一次，所以 join 有 phi。
// 把 entry 的分支改成"总是走 then"后，else 变死块，join 只剩一个前驱——
// 此时 phi 的实参必须跟着裁剪，否则会读到 else 分支的值。
func buildConstDiamond(t *testing.T, truthy bool) (*Function, *Block) {
	t.Helper()
	pool := testPool(object.Boolean{Value: truthy}, object.Int{Value: 1}, object.Int{Value: 2})
	fn := &Function{Name: "<t>", pool: pool}
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
		imm := 1
		if b == els {
			imm = 2
		}
		v := fn.NewValue()
		fn.Emit(b, &Instr{Op: OpConst, Dst: v, Imm: imm})
		fn.Emit(b, &Instr{Op: OpStoreLocal, Args: []ValueID{v}, Imm: 0})
		fn.Emit(b, &Instr{Op: OpJump, Imm: 3})
	}

	out := fn.NewValue()
	fn.Emit(join, &Instr{Op: OpLoadLocal, Dst: out, Imm: 0})
	fn.Emit(join, &Instr{Op: OpSink, Args: []ValueID{out}})
	fn.Emit(join, &Instr{Op: OpExit})

	fn.linkBlocks()
	return fn, join
}

// 把 terminator 改成无条件跳转后，删除不可达块时 phi 实参必须跟着裁剪。
//
// 注意 relink 只保证"实参与当前前驱一致"；某个前驱只有在对应的块被删除后
// 才会消失，所以这里要按真实管线顺序：折叠分支 -> 删不可达块 -> 检查 phi。
func TestRelinkPrunesPhiArgs(t *testing.T) {
	fn, join := buildConstDiamond(t, true)
	if err := fn.runSSA(); err != nil {
		t.Fatalf("runSSA: %v", err)
	}
	if len(join.Preds) != 2 {
		t.Fatalf("join should start with 2 preds, got %d", len(join.Preds))
	}
	var phi *Instr
	for _, in := range join.Instrs {
		if in.Op == OpPhi {
			phi = in
		}
	}
	if phi == nil || len(phi.Args) != 2 {
		t.Fatalf("want a phi with 2 args, got %+v", phi)
	}
	thenArg := phi.Args[predIndex(join, fn.Blocks[1])]

	// 模拟"分支折叠"：entry 改成无条件跳到 then
	last := fn.Entry.Last()
	last.Op = OpJump
	last.Imm = last.Imm2
	last.Args = nil
	if err := fn.relink(); err != nil {
		t.Fatalf("relink: %v", err)
	}
	// 此时 else 还占着一个前驱位置（它自己仍指向 join），必须先删块
	if !fn.removeUnreachable() {
		t.Fatal("expected the else block to become unreachable")
	}
	if len(join.Preds) != 1 {
		t.Fatalf("join should have 1 pred after pruning, got %d", len(join.Preds))
	}
	if len(phi.Args) != 1 {
		t.Fatalf("phi should be pruned to 1 arg, got %d", len(phi.Args))
	}
	// 保留下来的实参必须是 then 分支写入的那个值
	if phi.Args[0] != thenArg {
		t.Fatalf("phi kept the wrong argument: got %d, want %d (then branch)", phi.Args[0], thenArg)
	}
}

// 如果 join 多出一个前驱（而 phi 没有对应实参），relink 必须报错，
// 而不是按位置瞎填——错填会比放弃优化危险得多。
func TestRelinkRejectsUnexpectedPredecessor(t *testing.T) {
	fn, join := buildConstDiamond(t, true)
	if err := fn.runSSA(); err != nil {
		t.Fatalf("runSSA: %v", err)
	}
	extra := &Block{ID: len(fn.Blocks)}
	fn.Blocks = append(fn.Blocks, extra)
	fn.Emit(extra, &Instr{Op: OpJump, Imm: join.ID})

	if err := fn.relink(); err == nil {
		t.Fatal("relink should fail when a phi cannot supply an argument for a new predecessor")
	}
}

// 折叠之后死块必须真的被删掉（这是之前"忘记重连"导致的失效行为）。
func TestOptimizeRemovesBlockUnreachableByConstantBranch(t *testing.T) {
	fn, join := buildConstDiamond(t, false) // 条件恒假 -> 走 else，then 变死块
	if err := fn.runSSA(); err != nil {
		t.Fatalf("runSSA: %v", err)
	}
	if err := fn.optimize(); err != nil {
		t.Fatalf("optimize: %v", err)
	}
	// 只剩 entry / else / join 三块
	if len(fn.Blocks) != 3 {
		t.Fatalf("expected the dead block to be removed, got %d blocks", len(fn.Blocks))
	}
	if len(join.Preds) != 1 {
		t.Fatalf("join preds = %d, want 1", len(join.Preds))
	}
	for _, in := range join.Instrs {
		if in.Op == OpPhi && len(in.Args) != len(join.Preds) {
			t.Fatalf("phi args = %d, preds = %d", len(in.Args), len(join.Preds))
		}
	}
	if err := fn.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

// validate 必须能抓出"phi 实参与前驱不一致"这种结构性错误。
func TestValidateCatchesPhiDesync(t *testing.T) {
	fn, join := buildConstDiamond(t, true)
	if err := fn.runSSA(); err != nil {
		t.Fatalf("runSSA: %v", err)
	}
	// 人为制造错位：删掉一个实参但不改前驱
	for _, in := range join.Instrs {
		if in.Op == OpPhi {
			in.Args = in.Args[:1]
		}
	}
	if err := fn.validate(); err == nil {
		t.Fatal("validate should reject a phi whose arg count differs from pred count")
	}
}

// validate 必须能抓出"块末尾没有终止指令"。
func TestValidateCatchesMissingTerminator(t *testing.T) {
	fn, join := buildConstDiamond(t, true)
	if err := fn.runSSA(); err != nil {
		t.Fatalf("runSSA: %v", err)
	}
	join.Instrs = join.Instrs[:len(join.Instrs)-1] // 去掉 OpExit
	if err := fn.validate(); err == nil {
		t.Fatal("validate should reject a block without a terminator")
	}
}

// validate 必须能抓出"引用了没有定义点的值"。
func TestValidateCatchesUndefinedValue(t *testing.T) {
	fn := &Function{Name: "<t>", pool: testPool(&object.Int{Value: 1})}
	entry := &Block{ID: 0}
	fn.Blocks = []*Block{entry}
	fn.Entry = entry
	fn.Emit(entry, &Instr{Op: OpSink, Args: []ValueID{ValueID(9999)}})
	fn.Emit(entry, &Instr{Op: OpExit})
	fn.linkBlocks()
	if err := fn.validate(); err == nil {
		t.Fatal("validate should reject a use of an undefined value")
	}
}

// phi 出现在块中间也是非法结构。
func TestValidateCatchesMisplacedPhi(t *testing.T) {
	fn, join := buildConstDiamond(t, true)
	if err := fn.runSSA(); err != nil {
		t.Fatalf("runSSA: %v", err)
	}
	// 把 phi 挪到块尾
	var phi *Instr
	rest := join.Instrs[:0]
	for _, in := range join.Instrs {
		if in.Op == OpPhi {
			phi = in
			continue
		}
		rest = append(rest, in)
	}
	join.Instrs = append(rest, phi)
	if err := fn.validate(); err == nil {
		t.Fatal("validate should reject a phi that is not at the block head")
	}
}
