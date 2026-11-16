package ir

import (
	"Interpreter/code"
	"Interpreter/object"
)

// 本文件把优化后的 IR 降回栈字节码。
//
// 采用"树式代码生成"（tree-based codegen），这是栈机后端的常规做法：
//
//   - 若某个值只被用到一次、且使用点和定义点在同一个基本块里，
//     就直接在操作数栈上算出来，不落槽位；
//   - 其它值（多次使用 / 跨块 / phi / 形参）先算出来再 OpSetLocal，
//     用的时候 OpGetLocal。
//
// 这样 `a+b` 之类仍然是 GetLocal/GetLocal/Add，
// 不会退化成"每一步都往槽位里存一次"。

// splitCriticalEdges 为目标块带 phi 的边插入边块。
func (f *Function) splitCriticalEdges() {
	origin := len(f.Blocks)
	for i := 0; i < origin; i++ {
		b := f.Blocks[i]
		last := b.Last()
		if last == nil || last.Op != OpBranch {
			continue
		}
		for _, which := range []int{0, 1} {
			tid := last.Imm
			if which == 1 {
				tid = last.Imm2
			}
			if tid < 0 || tid >= len(f.Blocks) {
				continue
			}
			target := f.Blocks[tid]
			if !blockHasPhi(target) {
				continue
			}
			// 目标块多前驱要拆（避开并行拷贝串值）；
			// 当前块多后继也要拆（否则 phi 拷贝没有唯一落点）。
			if len(target.Preds) < 2 && len(b.Succs) < 2 {
				continue
			}
			edge := &Block{ID: len(f.Blocks), Offset: -1}
			edge.Instrs = []*Instr{{Op: OpJump, Imm: target.ID}}
			f.Blocks = append(f.Blocks, edge)

			for k, p := range target.Preds {
				if p == b {
					target.Preds[k] = edge
					break
				}
			}
			edge.Preds = []*Block{b}
			edge.Succs = []*Block{target}

			if which == 0 {
				last.Imm = edge.ID
			} else {
				last.Imm2 = edge.ID
			}
			for k, s := range b.Succs {
				if s == target {
					b.Succs[k] = edge
				}
			}
		}
	}
}

func blockHasPhi(b *Block) bool {
	for _, in := range b.Instrs {
		if in.Op == OpPhi {
			return true
		}
	}
	return false
}

// lower 生成最终字节码与行号表，返回需要的槽位总数。
func (f *Function) lower(isMain bool) (code.Instructions, []code.LineEntry, int, error) {
	f.splitCriticalEdges()
	numSlots := f.allocateSlots()

	defBlock := f.defBlocks()
	e := &emitter{
		fn:          f,
		isMain:      isMain,
		blockOffset: map[int]int{},
		useCount:    f.countUses(),
		defBlock:    defBlock,
		crossBlock:  f.crossBlockUses(defBlock),
	}
	for _, b := range f.Blocks {
		e.blockOffset[b.ID] = len(e.ins)
		if err := e.emitBlock(b); err != nil {
			return nil, nil, 0, err
		}
	}
	if err := e.patchJumps(); err != nil {
		return nil, nil, 0, err
	}
	// 主函数里的 OpExit 需要显式跳到末尾（见 emitTerminator），
	// 末尾偏移只有全部块都发射完才知道，所以在这里统一回填。
	end := len(e.ins)
	for _, pos := range e.endJumps {
		newIns := code.Make(code.Opcode(e.ins[pos]), end)
		copy(e.ins[pos:pos+len(newIns)], newIns)
	}
	return e.ins, e.lines, numSlots, nil
}

// countUses 统计每个值被引用的次数（含 phi 实参）。
func (f *Function) countUses() []int {
	uses := make([]int, int(f.numValue)+1)
	inc := func(v ValueID) {
		if v != NoValue && int(v) < len(uses) {
			uses[v]++
		}
	}
	for _, b := range f.Blocks {
		for _, in := range b.Instrs {
			for _, a := range in.Args {
				inc(a)
			}
		}
	}
	return uses
}

// defBlocks 返回每个值所在的定义块。
func (f *Function) defBlocks() []*Block {
	out := make([]*Block, int(f.numValue)+1)
	for _, b := range f.Blocks {
		for _, in := range b.Instrs {
			for _, dst := range []ValueID{in.Dst, in.Dst2} {
				if dst != NoValue && int(dst) < len(out) {
					out[dst] = b
				}
			}
		}
	}
	return out
}

// crossBlockUses 标记"使用点与定义点不在同一块"的值。
// 这些值必须落在槽位里，因为栈上的位置不能跨块保留。
func (f *Function) crossBlockUses(defBlock []*Block) []bool {
	out := make([]bool, int(f.numValue)+1)
	mark := func(v ValueID) {
		if v != NoValue && int(v) < len(out) {
			out[v] = true
		}
	}
	for _, b := range f.Blocks {
		for _, in := range b.Instrs {
			if in.Op == OpPhi {
				// phi 实参在边块（或前驱块末尾）被读到。
				// 只有当边没有被拆开、且实参就定义在这个前驱块里时，
				// 才能在发射拷贝时直接把它算到栈上，否则必须落槽位。
				for i, a := range in.Args {
					if a == NoValue || int(a) >= len(out) {
						continue
					}
					if i < len(b.Preds) && defBlock[a] == b.Preds[i] &&
						len(b.Preds[i].Succs) == 1 &&
						f.Def(a) != nil && f.Def(a).Op != OpPhi && f.Def(a).Op != OpInit {
						continue
					}
					mark(a)
				}
				continue
			}
			for _, a := range in.Args {
				if a == NoValue || int(a) >= len(out) {
					continue
				}
				if d := f.Def(a); d != nil && (d.Op == OpPhi || d.Op == OpInit) {
					mark(a)
					continue
				}
				if defBlock[a] != b {
					mark(a)
				}
			}
		}
	}
	return out
}

type jumpPatch struct {
	pos    int
	target int
}

type emitter struct {
	fn          *Function
	isMain      bool
	ins         code.Instructions
	lines       []code.LineEntry
	patches     []jumpPatch
	blockOffset map[int]int
	curLine     int
	useCount    []int
	defBlock    []*Block
	crossBlock  []bool
	// endJumps 记录"跳到函数末尾"的合成跳转（只在主函数的 OpExit 上出现），
	// 位置在 lower() 发射完所有块之后统一回填。
	endJumps []int
}

func (e *emitter) emit(op code.Opcode, operands ...int) {
	pos := len(e.ins)
	e.ins = append(e.ins, code.Make(op, operands...)...)
	if e.curLine > 0 {
		if len(e.lines) == 0 || e.lines[len(e.lines)-1].Line != e.curLine {
			e.lines = append(e.lines, code.LineEntry{Offset: pos, Line: e.curLine})
		}
	}
}

func (e *emitter) emitJump(op code.Opcode, targetBlock int) {
	pos := len(e.ins)
	e.ins = append(e.ins, code.Make(op, 0)...)
	e.patches = append(e.patches, jumpPatch{pos: pos, target: targetBlock})
}

func (e *emitter) slot(v ValueID) (int, error) {
	s := e.fn.Slot(v)
	if s < 0 {
		return 0, bail("value %d has no slot", v)
	}
	return s, nil
}

// slotOnly 判断某个值是否必须先落槽位。
func (e *emitter) slotOnly(v ValueID) bool {
	if v == NoValue {
		return true
	}
	if e.useCount[v] != 1 {
		return true
	}
	d := e.fn.Def(v)
	if d == nil {
		return true
	}
	switch d.Op {
	case OpConst, OpPhi, OpInit:
		return true
	}
	if int(v) < len(e.crossBlock) && e.crossBlock[v] {
		return true
	}
	return false
}

// emitConst 发射常量。
//
// null 与布尔有 1 字节的专用指令（OpNull/OpTrue/OpFalse），
// 比 3 字节的 OpConstant 短，所以降级时优先用它们。
func (e *emitter) emitConst(idx int) {
	switch c := e.fn.Const(idx).(type) {
	case object.Null:
		e.emit(code.OpNull)
	case object.Boolean:
		if c.Value {
			e.emit(code.OpTrue)
		} else {
			e.emit(code.OpFalse)
		}
	default:
		e.emit(code.OpConstant, idx)
	}
}

// loadValue 把值放到操作数栈上。
func (e *emitter) loadValue(v ValueID) error {
	d := e.fn.Def(v)
	if d != nil && d.Op == OpConst {
		// 常量不占槽位，直接重新发射
		e.emitConst(d.Imm)
		return nil
	}
	s, err := e.slot(v)
	if err != nil {
		return err
	}
	e.emit(code.OpGetLocal, s)
	return nil
}

// emitValue 把值算到操作数栈顶：能在栈上直算的就地展开，否则从槽位读。
func (e *emitter) emitValue(v ValueID, done map[ValueID]bool) error {
	if v == NoValue {
		return nil
	}
	d := e.fn.Def(v)
	if d == nil || e.slotOnly(v) || done[v] || d.Op == OpNop {
		return e.loadValue(v)
	}
	savedLine := e.curLine
	if d.Line > 0 && !d.CannotTrap() {
		e.curLine = d.Line
	}
	defer func() { e.curLine = savedLine }()
	done[v] = true
	// 常量 callee 的调用：callee 不压栈，直接按常量发调用
	if fnIdx, ok := e.constCallee(d); ok {
		for _, a := range d.Args[1:] {
			if err := e.emitValue(a, done); err != nil {
				return err
			}
		}
		e.emit(code.OpCallFuncConst, fnIdx, len(d.Args)-1)
		return nil
	}
	for _, a := range d.Args {
		if err := e.emitValue(a, done); err != nil {
			return err
		}
	}
	return e.emitOp(d)
}

// constCallee 判断一次调用的 callee 是否来自常量池里的函数对象，且只被这一次
// 调用使用。满足时可以直接发 OpCallFuncConst，把 callee 从操作数栈上完全省掉
// —— 每次调用少一条 OpClosure（一条指令 + 一次接口写栈 + 一次读栈）。
func (e *emitter) constCallee(in *Instr) (int, bool) {
	if in.Op != OpCall || len(in.Args) == 0 {
		return 0, false
	}
	callee := in.Args[0]
	if callee == NoValue || int(callee) >= len(e.useCount) || e.useCount[callee] != 1 {
		return 0, false
	}
	d := e.fn.Def(callee)
	if d == nil || d.Op != OpMakeClosure {
		return 0, false
	}
	return d.Imm, true
}

// callOperands 返回"需要压栈"的实参：常量 callee 不占栈，所以要从 Args[0] 之后算起。
func (e *emitter) callOperands(in *Instr) []ValueID {
	if _, ok := e.constCallee(in); ok {
		return in.Args[1:]
	}
	return in.Args
}

// firstUses 返回每个值在本块中第一次被使用的位置（不在本块中使用时是块尾）。
func firstUses(b *Block) map[ValueID]int {
	m := make(map[ValueID]int, len(b.Instrs))
	for i, in := range b.Instrs {
		for _, a := range in.Args {
			if a == NoValue {
				continue
			}
			if _, ok := m[a]; !ok {
				m[a] = i
			}
		}
	}
	return m
}

// mayWriteFlags 标记哪些指令"一定就地发射、且可能改写堆上数据"。
//
// 只有这类指令才能构成"延迟展开"的屏障：
//   - OpUpdate / OpStoreGlobal / OpMethodCall 永远就地发射；
//   - OpCall 只有在必须物化（多次使用、或结果被丢弃）时才就地发射；
//     单次使用的调用会跟着使用者一起延迟，两者的相对顺序不变，
//     所以它不是屏障（否则每一次 `f()+g()` 都要多一次存-取）。
func (e *emitter) mayWriteFlags(b *Block) []bool {
	out := make([]bool, len(b.Instrs))
	for i, in := range b.Instrs {
		switch in.Op {
		case OpUpdate, OpStoreGlobal, OpMethodCall:
			out[i] = true
		case OpCall:
			out[i] = in.Dst == NoValue || e.useCount[in.Dst] > 1
		}
	}
	return out
}

// callFlags 标记所有调用点。
//
// 下标读取（OpIndex）比普通计算更敏感：延迟展开会**重新读一次内存**，
// 而中间的调用可能已经改写了容器。所以对 OpIndex 而言，任何调用都是屏障，
// 与它是否被物化无关。
func callFlags(b *Block) []bool {
	out := make([]bool, len(b.Instrs))
	for i, in := range b.Instrs {
		if in.Op == OpCall || in.Op == OpMethodCall {
			out[i] = true
		}
	}
	return out
}

// anyBetween 判断 (from, to) 之间是否有被标记的指令。
// 区间右端是开区间：使用点自身那条指令的写入发生在参数求值之后，不影响展开。
func anyBetween(flags []bool, from, to int) bool {
	if to <= from {
		return false
	}
	if to > len(flags) {
		to = len(flags)
	}
	for i := from + 1; i < to; i++ {
		if flags[i] {
			return true
		}
	}
	return false
}

// emitBlock 发射一个基本块。
//
// "单次使用就地展开"是这里的核心优化，但它有一个必须显式排除的陷阱：
// 如果某个值的使用点在**另一次写内存之后**，重新展开会读到更新后的内容。
// 典型反例（真实踩过）：
//
//	var t = a[i]      // 展开成 读 a[i]
//	a[i] = a[j]       // 写内存
//	a[j] = t          // 若在这里重新展开 t，读到的是**新**的 a[i]
//
// 所以下面用"第一次使用位置 + 中间的写标记"来判定能不能展开。
func (e *emitter) emitBlock(b *Block) error {
	done := map[ValueID]bool{}
	useAt := firstUses(b)
	writes := e.mayWriteFlags(b)
	calls := callFlags(b)

	for idx, in := range b.Instrs {
		if in.Line > 0 {
			e.curLine = in.Line
		}
		if in.IsTerminator() {
			return e.emitTerminator(b, in, done)
		}
		if in.Op == OpPhi || in.Op == OpInit || in.Op == OpNop || in.Op == OpConst {
			// OpConst 不落槽位：用到它的地方会重新发射 OpConstant
			continue
		}

		if in.Op == OpMethodCall {
			for _, a := range in.Args {
				if err := e.emitValue(a, done); err != nil {
					return err
				}
			}
			if err := e.emitOp(in); err != nil {
				return err
			}
			// OpPop changes lastPop; even unused results must be consumed by a store.
			for _, dst := range []ValueID{in.Dst2, in.Dst} {
				s, err := e.slot(dst)
				if err != nil {
					return err
				}
				e.emit(code.OpSetLocal, s)
			}
			done[in.Dst] = true
			done[in.Dst2] = true
			continue
		}

		if in.Dst == NoValue {
			// 无结果的指令：先把实参算到栈上，再执行
			for _, a := range e.callOperands(in) {
				if err := e.emitValue(a, done); err != nil {
					return err
				}
			}
			if err := e.emitOp(in); err != nil {
				return err
			}
			continue
		}

		if e.useCount[in.Dst] == 0 {
			// 结果没人要：既无副作用也不会报错的指令直接跳过
			// （DCE 本该删掉，这里兜底），其余仍然要执行。
			if in.CannotTrap() {
				done[in.Dst] = true
				continue
			}
			for _, a := range e.callOperands(in) {
				if err := e.emitValue(a, done); err != nil {
					return err
				}
			}
			if err := e.emitOp(in); err != nil {
				return err
			}
			e.emit(code.OpPop)
			done[in.Dst] = true
			continue
		}

		if !e.slotOnly(in.Dst) {
			// 单次使用：只有在"展开之前不会有写内存"时才允许推迟到使用点。
			// 使用点在本块外（例如作为 phi 实参，在块尾才被读到）时，
			// 一律按块尾处理——否则一次推迟就会把副作用搬到写操作之后。
			use, ok := useAt[in.Dst]
			if !ok {
				use = len(b.Instrs)
			}
			unsafe := anyBetween(writes, idx, use)
			if !unsafe && in.Op == OpIndex {
				// 下标读会被"重新读一次内存"，中间的调用可能已经改写了容器
				unsafe = anyBetween(calls, idx, use)
			}
			if !unsafe {
				continue
			}
		}

		// 需要落槽位
		for _, a := range e.callOperands(in) {
			if err := e.emitValue(a, done); err != nil {
				return err
			}
		}
		if err := e.emitOp(in); err != nil {
			return err
		}
		s, err := e.slot(in.Dst)
		if err != nil {
			return err
		}
		e.emit(code.OpSetLocal, s)
		done[in.Dst] = true
	}

	if err := e.emitPhiCopies(b, done); err != nil {
		return err
	}
	if !e.isMain {
		e.emit(code.OpNull)
		e.emit(code.OpReturnVal)
	}
	return nil
}

func (e *emitter) emitTerminator(b *Block, in *Instr, done map[ValueID]bool) error {
	if err := e.emitPhiCopies(b, done); err != nil {
		return err
	}
	switch in.Op {
	case OpJump:
		e.emitJump(code.OpJump, in.Imm)

	case OpBranch:
		if len(in.Args) != 1 {
			return bail("branch with %d operands", len(in.Args))
		}
		if err := e.emitValue(in.Args[0], done); err != nil {
			return err
		}
		e.emitJump(code.OpJumpNotTrue, in.Imm)
		e.emitJump(code.OpJump, in.Imm2)

	case OpReturn:
		if len(in.Args) != 1 {
			return bail("return with %d operands", len(in.Args))
		}
		if err := e.emitValue(in.Args[0], done); err != nil {
			return err
		}
		e.emit(code.OpReturnVal)

	case OpExit:
		if e.isMain {
			// 主函数里的"退出"就是执行到脚本末尾。只有最后一个块可以自然
			// 落出；其它块必须显式跳到末尾，否则会顺序执行布局上紧随其后的
			// 块——`if(f()){1}else{0}` 作为脚本最后一句时，then 分支就会
			// 继续执行 else 分支的代码，把最终结果改写成 else 的值。
			if len(e.fn.Blocks) > 0 && b != e.fn.Blocks[len(e.fn.Blocks)-1] {
				e.endJumps = append(e.endJumps, len(e.ins))
				e.ins = append(e.ins, code.Make(code.OpJump, 0)...)
			}
			return nil
		}
		e.emit(code.OpNull)
		e.emit(code.OpReturnVal)
	}
	return nil
}

// emitPhiCopies 为后继块的 phi 填值。
//
// 这是**并行拷贝**（parallel copy）：所有实参必须在任何一次写回之前求值。
// 逐条"读-写、读-写"是错的：
//
//	# i、s 都是循环携带变量，实参分别是 i+1 和 s+i
//	SetLocal i_slot ← i+1     # 覆盖了 i
//	... 展开 s+i 时读到的 i 已经是 i+1 —— 结果多算一轮（实测 10 变成 15）
//
// 因此分两趟：先把所有实参压到操作数栈（纯读），再按相反顺序写回。
// 实参若被多个 phi 使用，useCount > 1，slotOnly 会把它安排在槽位里，
// 这里读的就是槽位；单次使用的实参只属于这一个 phi，不会重复展开。
func (e *emitter) emitPhiCopies(b *Block, done map[ValueID]bool) error {
	if len(b.Succs) != 1 {
		return nil
	}
	s := b.Succs[0]
	idx := predIndex(s, b)
	if idx < 0 {
		return nil
	}

	type copy struct {
		arg ValueID
		dst ValueID
	}
	copies := make([]copy, 0, len(s.Instrs))
	for _, in := range s.Instrs {
		if in.Op != OpPhi {
			continue
		}
		if idx >= len(in.Args) {
			return bail("phi at block %d missing operand for edge from block %d", s.ID, b.ID)
		}
		arg := in.Args[idx]
		if arg == in.Dst {
			continue
		}
		// 实参已经在目标槽位里（例如循环里被原地修改的容器，实参和 phi
		// 分到同一个槽位）：这条拷贝是恒等拷贝，读一遍再写回去纯属浪费。
		// 全局变量的值没有槽位（Slot 返回 -1），不做这个判断。
		if e.slotOnly(arg) {
			if from, to := e.fn.Slot(arg), e.fn.Slot(in.Dst); from >= 0 && from == to {
				continue
			}
		}
		copies = append(copies, copy{arg: arg, dst: in.Dst})
	}

	// 第一趟：只读。所有实参依次压栈。
	for _, c := range copies {
		if err := e.emitValue(c.arg, done); err != nil {
			return err
		}
	}
	// 第二趟：只写。栈顶依次是最后一个实参。
	for i := len(copies) - 1; i >= 0; i-- {
		dstSlot, err := e.slot(copies[i].dst)
		if err != nil {
			return err
		}
		e.emit(code.OpSetLocal, dstSlot)
	}
	return nil
}

// emitOp 只发射"操作本身"，实参已经在栈上，结果留在栈顶。
func (e *emitter) emitOp(in *Instr) error {
	switch in.Op {
	case OpNop, OpPhi, OpInit, OpConst:
		// OpInit：形参本来就在槽位里
		// OpConst：由 loadValue/emitValue 按需重新发射
		return nil

	case OpBin, OpUn:
		e.emit(code.Opcode(in.Imm))

	case OpBuiltin:
		e.emit(code.OpGetBuiltin, in.Imm)

	case OpMakeArray:
		e.emit(code.OpBuildArray, len(in.Args))

	case OpMakeMap:
		e.emit(code.OpMakeMap, len(in.Args))

	case OpIndex:
		e.emit(code.OpIndex)

	case OpMakeSlice:
		e.emit(code.OpMakeSlice)

	case OpUpdate:
		e.emit(code.OpUpdate)
		e.emit(code.OpPop) // VM 会把容器压回栈，这里不需要

	case OpMakeClosure:
		e.emit(code.OpClosure, in.Imm)

	case OpCall:
		if len(in.Args) == 0 {
			return bail("call without callee")
		}
		// callee 是常量池里的函数且只被这次调用使用：直接按常量调用
		if fnIdx, ok := e.constCallee(in); ok {
			e.emit(code.OpCallFuncConst, fnIdx, len(in.Args)-1)
			return nil
		}
		e.emit(code.OpCallFunc, len(in.Args)-1)

	case OpMethodCall:
		if len(in.Args) == 0 {
			return bail("method call without receiver")
		}
		e.emit(code.OpCallMethod, in.Imm, len(in.Args)-1)

	case OpLoadGlobal:
		e.emit(code.OpGetGlobal, in.Imm)

	case OpStoreGlobal:
		e.emit(code.OpSetGlobal, in.Imm)

	case OpSink:
		e.emit(code.OpPop)

	default:
		return bail("cannot lower op %s", in.Op)
	}
	return nil
}

func (e *emitter) patchJumps() error {
	for _, p := range e.patches {
		target, ok := e.blockOffset[p.target]
		if !ok {
			return bail("jump to unknown block %d", p.target)
		}
		newIns := code.Make(code.Opcode(e.ins[p.pos]), target)
		copy(e.ins[p.pos:p.pos+len(newIns)], newIns)
	}
	return nil
}
