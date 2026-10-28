package code

import "fmt"

// 本文件实现"超指令"（superinstruction）融合。
//
// 背景：解释器的每条指令都要付一次派发成本（读 opcode、跳转表间接跳转、
// 读操作数、栈指针调整）。像 `a < 10` 这样一句话在栈式字节码里是
//
//	OpGetLocal a ; OpConstant 10 ; OpLT
//
// 三条指令、三次派发，而它要做的只是一次比较。把这类"固定搭配"合并成
// 一条融合指令，就能把三次派发压成一次。这也是 CPython 3.11+ 专用化指令、
// JVM 的 _fast 指令族的同一思路。
//
// 设计原则：
//   - 只减少派发，不改变语义：融合指令做的就是原序列做的事，求值顺序、
//     错误信息、行号全部保持一致；
//   - 融合后长度不变或更短，因此不会让代码变大；
//   - 任何可疑情况（跳转目标落在序列中间、跨源码行、未知操作码、
//     跳转目标无法映射）都放弃该处融合，最坏情况是原样返回。
//
// 安全约束（缺一不可）：
//  1. 被合并的序列里，除第一条外不能是跳转目标 —— 否则跳到中间会错位；
//  2. 被合并的指令必须来自同一源码行 —— 否则运行时错误报的行号会变；
//  3. 跳转目标偏移变化后必须重新映射。

// inst 是解码后的一条指令。
type inst struct {
	offset   int
	op       Opcode
	operands []int
	size     int
}

// decode 把字节流解码成定长指令列表。
func decode(ins Instructions) ([]inst, error) {
	out := make([]inst, 0, len(ins)/2)
	for off := 0; off < len(ins); {
		op := Opcode(ins[off])
		def, ok := Definitions[op]
		if !ok {
			return nil, fmt.Errorf("code: unknown opcode %d at offset %d", ins[off], off)
		}
		width := 0
		for _, w := range def.OperandWidth {
			width += w
		}
		if off+1+width > len(ins) {
			return nil, fmt.Errorf("code: truncated instruction %s at offset %d", def.Name, off)
		}
		operands, _ := ReadOperand(def, ins[off+1:])
		out = append(out, inst{offset: off, op: op, operands: operands, size: 1 + width})
		off += 1 + width
	}
	return out, nil
}

// operandKind 判断某条指令是不是"把一个操作数压栈"，并返回它的来源类型。
func operandKind(op Opcode) (byte, bool) {
	switch op {
	case OpGetLocal:
		return OperandLocal, true
	case OpGetGlobal:
		return OperandGlobal, true
	case OpConstant:
		return OperandConst, true
	}
	return 0, false
}

// isBinaryOp 判断是否是融合规则接受的二元运算（算术或比较）。
func isBinaryOp(op Opcode) bool {
	return IsArithOp(op) || IsCompareOp(op)
}

// fuseContext 是一次融合过程中共用的只读信息。
type fuseContext struct {
	insts    []inst
	isTarget []bool
	lineOf   func(offset int) int
}

// contiguous 报告 insts[i] 与 insts[i+1] 是否物理相邻。
func (c *fuseContext) contiguous(i int) bool {
	return i+1 < len(c.insts) && c.insts[i].offset+c.insts[i].size == c.insts[i+1].offset
}

// reusable 报告 insts[i] 能不能作为融合序列的"非首条"指令：
// 它不能是跳转目标，且必须与首条同一源码行。
func (c *fuseContext) reusable(head, i int) bool {
	if c.isTarget[c.insts[i].offset] {
		return false
	}
	return c.lineOf(c.insts[head].offset) == c.lineOf(c.insts[i].offset)
}

// tryFuse 尝试在 i 处融合，返回融合后的指令字节与被合并的指令条数。
func (c *fuseContext) tryFuse(i int) (Instructions, int, bool) {
	// 规则 1（最长优先）：操作数 + 操作数 + 二元运算
	if fused, ok := c.fuseOperandOperandBin(i); ok {
		return fused, 3, true
	}
	// 规则 2：操作数 + 返回
	if fused, ok := c.fuseOperandReturn(i); ok {
		return fused, 2, true
	}
	// 规则 3：二元运算 + 返回
	if fused, ok := c.fuseBinReturn(i); ok {
		return fused, 2, true
	}
	return nil, 0, false
}

// fuseOperandOperandBin 匹配 OpGetLocal/OpGetGlobal/OpConstant 两条 + 二元运算。
//
//	原序列 3+3+1 = 7 字节、3 次派发
//	融合后 1+1+1+2+2 = 7 字节、1 次派发
func (c *fuseContext) fuseOperandOperandBin(i int) (Instructions, bool) {
	if i+2 >= len(c.insts) || !c.contiguous(i) || !c.contiguous(i+1) {
		return nil, false
	}
	kindA, okA := operandKind(c.insts[i].op)
	if !okA {
		return nil, false
	}
	kindB, okB := operandKind(c.insts[i+1].op)
	if !okB {
		return nil, false
	}
	op := c.insts[i+2].op
	if !isBinaryOp(op) {
		return nil, false
	}
	if !c.reusable(i, i+1) || !c.reusable(i, i+2) {
		return nil, false
	}
	return Make(OpBinFused,
		int(kindA<<4|kindB),
		int(op),
		c.insts[i].operands[0],
		c.insts[i+1].operands[0],
	), true
}

// fuseOperandReturn 匹配"压入一个操作数 + 返回"。
//
//	原序列 3+1 = 4 字节、2 次派发；融合后 1+1+2 = 4 字节、1 次派发
func (c *fuseContext) fuseOperandReturn(i int) (Instructions, bool) {
	if !c.contiguous(i) {
		return nil, false
	}
	kind, ok := operandKind(c.insts[i].op)
	if !ok || c.insts[i+1].op != OpReturnVal {
		return nil, false
	}
	if !c.reusable(i, i+1) {
		return nil, false
	}
	return Make(OpReturnOperand, int(kind), c.insts[i].operands[0]), true
}

// fuseBinReturn 匹配"二元运算 + 返回"。
//
//	原序列 1+1 = 2 字节、2 次派发；融合后 1+1 = 2 字节、1 次派发
func (c *fuseContext) fuseBinReturn(i int) (Instructions, bool) {
	if !c.contiguous(i) {
		return nil, false
	}
	if !isBinaryOp(c.insts[i].op) || c.insts[i+1].op != OpReturnVal {
		return nil, false
	}
	if !c.reusable(i, i+1) {
		return nil, false
	}
	return Make(OpBinReturn, int(c.insts[i].op)), true
}

// jumpPatch 记录一条待改写的跳转。
type jumpPatch struct {
	pos    int // 跳转指令在新流里的起始偏移
	target int // 原流里的目标偏移
}

// Fuse 把常见的相邻指令合并成融合指令。
//
// 返回的指令流与原流**逐条等价**（同样的求值顺序、同样的错误、同样的行号），
// 只是派发次数更少。没有任何可融合之处时原样返回入参。
//
// 出错时返回原始输入与错误：调用方应当直接保留原指令流，不影响正确性。
func Fuse(ins Instructions, lines []LineEntry) (Instructions, []LineEntry, error) {
	if len(ins) == 0 {
		return ins, lines, nil
	}
	insts, err := decode(ins)
	if err != nil {
		return ins, lines, err
	}

	// 标记所有跳转目标（含入口）
	isTarget := make([]bool, len(ins))
	isTarget[0] = true
	for _, it := range insts {
		if it.op != OpJump && it.op != OpJumpNotTrue {
			continue
		}
		if len(it.operands) == 0 {
			return ins, lines, fmt.Errorf("code: jump at offset %d has no operand", it.offset)
		}
		t := it.operands[0]
		if t < 0 || t >= len(ins) {
			return ins, lines, fmt.Errorf("code: jump at offset %d targets %d, out of range", it.offset, t)
		}
		isTarget[t] = true
	}

	ctx := &fuseContext{
		insts:    insts,
		isTarget: isTarget,
		lineOf:   func(offset int) int { return LineAt(lines, offset) },
	}

	out := make(Instructions, 0, len(ins))
	outLines := make([]LineEntry, 0, len(lines))
	newOffset := make(map[int]int, len(insts))
	var patches []jumpPatch
	fused := 0

	for i := 0; i < len(insts); {
		if blob, consumed, ok := ctx.tryFuse(i); ok {
			head := insts[i].offset
			newOffset[head] = len(out)
			// 被吞掉的指令映射到融合指令起点：如果真有人跳到它们，
			// 也只会跳到这条等价指令的开头（isTarget 已保证不会发生）
			for k := 1; k < consumed; k++ {
				newOffset[insts[i+k].offset] = len(out)
			}
			out = append(out, blob...)
			appendLine(&outLines, lines, head, len(out)-len(blob))
			fused++
			i += consumed
			continue
		}

		it := insts[i]
		newOffset[it.offset] = len(out)
		if it.op == OpJump || it.op == OpJumpNotTrue {
			patches = append(patches, jumpPatch{pos: len(out), target: it.operands[0]})
		}
		out = append(out, ins[it.offset:it.offset+it.size]...)
		appendLine(&outLines, lines, it.offset, len(out)-it.size)
		i++
	}

	if fused == 0 {
		return ins, lines, nil
	}

	for _, p := range patches {
		to, ok := newOffset[p.target]
		if !ok {
			return ins, lines, fmt.Errorf("code: jump target %d has no new offset", p.target)
		}
		op := Opcode(out[p.pos])
		rewritten := Make(op, to)
		copy(out[p.pos:p.pos+len(rewritten)], rewritten)
	}
	return out, outLines, nil
}

// appendLine 把旧行号表里的行号搬到新偏移上，并合并连续相同的行号
// （与编译器生成行号表时的做法一致）。
func appendLine(out *[]LineEntry, lines []LineEntry, oldOffset, newOffset int) {
	line := LineAt(lines, oldOffset)
	if line <= 0 {
		return
	}
	if n := len(*out); n > 0 && (*out)[n-1].Line == line {
		return
	}
	*out = append(*out, LineEntry{Offset: newOffset, Line: line})
}
