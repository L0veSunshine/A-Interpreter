package code

import (
	"encoding/binary"
	"fmt"
)

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

// fusion 描述一次融合的结果。
type fusion struct {
	blob     Instructions
	consumed int
	// patchOff >= 0 时，blob 里偏移 patchOff 处的 u16 是跳转目标，
	// 需要在重排偏移后被改写成 patchTarget 的新位置。
	patchOff    int
	patchTarget int
}

// tryFuse 尝试在 i 处融合，返回融合结果与被合并的指令条数。
func (c *fuseContext) tryFuse(i int) (fusion, bool) {
	// 长模式优先：比较 + 条件分支
	if f, ok := c.fuseCmpBranch(i); ok {
		return f, true
	}
	// 下标由算术运算产生：必须先于 "操作数 + 操作数 + 二元运算" 尝试，
	// 否则短模式会吞掉算术，留下孤立的 OpIndex / OpUpdate。
	if f, ok := c.fuseKeyedBin(i, OpIndexBinFused, OpIndex); ok {
		return f, true
	}
	if f, ok := c.fuseKeyedBin(i, OpUpdateBinFused, OpUpdate); ok {
		return f, true
	}
	// 操作数 + 操作数 + 二元运算
	if f, ok := c.fuseOperandOperandBin(i); ok {
		return f, true
	}
	// 操作数 + 返回
	if f, ok := c.fuseOperandReturn(i); ok {
		return f, true
	}
	// 二元运算 + 返回
	if f, ok := c.fuseBinReturn(i); ok {
		return f, true
	}
	return fusion{}, false
}

// fuseCmpBranch 匹配"两个操作数 + 比较 + 条件跳转"。
//
// 这是条件判断最常见的形状（`if (a < 10) {...}` / `for (...; i < n; ...)`）。
// 合并之后布尔结果完全不需要物化到操作数栈上——省掉一次接口写栈、
// 一次读栈，以及一次 objToNativeBool 转换。
//
//	原序列 3+3+1+3 = 10 字节、4 次派发
//	融合后 1+1+1+2+2+2 = 9 字节、1 次派发
func (c *fuseContext) fuseCmpBranch(i int) (fusion, bool) {
	if i+3 >= len(c.insts) {
		return fusion{}, false
	}
	for k := 0; k < 3; k++ {
		if !c.contiguous(i + k) {
			return fusion{}, false
		}
	}
	kindA, okA := operandKind(c.insts[i].op)
	kindB, okB := operandKind(c.insts[i+1].op)
	if !okA || !okB {
		return fusion{}, false
	}
	if !IsCompareOp(c.insts[i+2].op) || c.insts[i+3].op != OpJumpNotTrue {
		return fusion{}, false
	}
	for k := 1; k <= 3; k++ {
		if !c.reusable(i, i+k) {
			return fusion{}, false
		}
	}
	blob := Make(OpCmpBranch,
		int(kindA<<4|kindB),
		int(c.insts[i+2].op),
		c.insts[i].operands[0],
		c.insts[i+1].operands[0],
		0, // 目标先占位，等所有偏移算完再回填
	)
	return fusion{
		blob:        blob,
		consumed:    4,
		patchOff:    7, // opcode(1) + kindAB(1) + cmpOp(1) + idxA(2) + idxB(2)
		patchTarget: c.insts[i+3].operands[0],
	}, true
}

// fuseKeyedBin 匹配 "操作数 A + 操作数 B + 算术运算 + tail"，tail 是 OpIndex 或 OpUpdate。
//
// 下标表达式（如 `dp[base + j]`）在原序列里要先把中间结果装箱压栈再弹出；
// 融合后中间结果直接以原生整数参与下标计算。
//
//	原序列 3+3+1+1 = 8 字节、4 次派发
//	融合后 1+1+1+2+2 = 7 字节、1 次派发
func (c *fuseContext) fuseKeyedBin(i int, fused, tail Opcode) (fusion, bool) {
	if i+3 >= len(c.insts) {
		return fusion{}, false
	}
	for k := 0; k < 3; k++ {
		if !c.contiguous(i + k) {
			return fusion{}, false
		}
	}
	kindA, okA := operandKind(c.insts[i].op)
	kindB, okB := operandKind(c.insts[i+1].op)
	if !okA || !okB {
		return fusion{}, false
	}
	if !IsArithOp(c.insts[i+2].op) || c.insts[i+3].op != tail {
		return fusion{}, false
	}
	for k := 1; k <= 3; k++ {
		if !c.reusable(i, i+k) {
			return fusion{}, false
		}
	}
	return fusion{
		blob: Make(fused,
			int(kindA<<4|kindB),
			int(c.insts[i+2].op),
			c.insts[i].operands[0],
			c.insts[i+1].operands[0],
		),
		consumed: 4,
		patchOff: -1,
	}, true
}

// fuseOperandOperandBin 匹配 OpGetLocal/OpGetGlobal/OpConstant 两条 + 二元运算。
//
//	原序列 3+3+1 = 7 字节、3 次派发
//	融合后 1+1+1+2+2 = 7 字节、1 次派发
func (c *fuseContext) fuseOperandOperandBin(i int) (fusion, bool) {
	if i+2 >= len(c.insts) || !c.contiguous(i) || !c.contiguous(i+1) {
		return fusion{}, false
	}
	kindA, okA := operandKind(c.insts[i].op)
	if !okA {
		return fusion{}, false
	}
	kindB, okB := operandKind(c.insts[i+1].op)
	if !okB {
		return fusion{}, false
	}
	op := c.insts[i+2].op
	if !isBinaryOp(op) {
		return fusion{}, false
	}
	if !c.reusable(i, i+1) || !c.reusable(i, i+2) {
		return fusion{}, false
	}
	return fusion{
		blob: Make(OpBinFused,
			int(kindA<<4|kindB),
			int(op),
			c.insts[i].operands[0],
			c.insts[i+1].operands[0],
		),
		consumed: 3,
		patchOff: -1,
	}, true
}

// fuseOperandReturn 匹配"压入一个操作数 + 返回"。
//
//	原序列 3+1 = 4 字节、2 次派发；融合后 1+1+2 = 4 字节、1 次派发
func (c *fuseContext) fuseOperandReturn(i int) (fusion, bool) {
	if !c.contiguous(i) {
		return fusion{}, false
	}
	kind, ok := operandKind(c.insts[i].op)
	if !ok || c.insts[i+1].op != OpReturnVal {
		return fusion{}, false
	}
	if !c.reusable(i, i+1) {
		return fusion{}, false
	}
	return fusion{
		blob:     Make(OpReturnOperand, int(kind), c.insts[i].operands[0]),
		consumed: 2,
		patchOff: -1,
	}, true
}

// fuseBinReturn 匹配"二元运算 + 返回"。
//
//	原序列 1+1 = 2 字节、2 次派发；融合后 1+1 = 2 字节、1 次派发
func (c *fuseContext) fuseBinReturn(i int) (fusion, bool) {
	if !c.contiguous(i) {
		return fusion{}, false
	}
	if !isBinaryOp(c.insts[i].op) || c.insts[i+1].op != OpReturnVal {
		return fusion{}, false
	}
	if !c.reusable(i, i+1) {
		return fusion{}, false
	}
	return fusion{
		blob:     Make(OpBinReturn, int(c.insts[i].op)),
		consumed: 2,
		patchOff: -1,
	}, true
}

// jumpPatch 记录一条待改写的跳转。
type jumpPatch struct {
	pos        int // 跳转指令（或含跳转的融合指令）在新流里的起始偏移
	operandOff int // 目标操作数相对 pos 的偏移
	target     int // 原流里的目标偏移
}

// patch 把一条 2 字节跳转目标写到新流里。
func (p jumpPatch) apply(out Instructions, newOffset map[int]int) error {
	to, ok := newOffset[p.target]
	if !ok {
		return fmt.Errorf("code: jump target %d has no new offset", p.target)
	}
	binary.BigEndian.PutUint16(out[p.pos+p.operandOff:], uint16(to))
	return nil
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
		var target int
		switch it.op {
		case OpJump, OpJumpNotTrue:
			target = it.operands[0]
		case OpCmpBranch:
			// 融合指令自带一个跳转目标（最后一个操作数）
			target = it.operands[len(it.operands)-1]
		default:
			continue
		}
		if len(it.operands) == 0 {
			return ins, lines, fmt.Errorf("code: jump at offset %d has no operand", it.offset)
		}
		if target < 0 || target >= len(ins) {
			return ins, lines, fmt.Errorf("code: jump at offset %d targets %d, out of range", it.offset, target)
		}
		isTarget[target] = true
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
		if f, ok := ctx.tryFuse(i); ok {
			head := insts[i].offset
			newOffset[head] = len(out)
			// 被吞掉的指令映射到融合指令起点：如果真有人跳到它们，
			// 也只会跳到这条等价指令的开头（isTarget 已保证不会发生）
			for k := 1; k < f.consumed; k++ {
				newOffset[insts[i+k].offset] = len(out)
			}
			if f.patchOff >= 0 {
				patches = append(patches, jumpPatch{
					pos:        len(out),
					operandOff: f.patchOff,
					target:     f.patchTarget,
				})
			}
			out = append(out, f.blob...)
			appendLine(&outLines, lines, head, len(out)-len(f.blob))
			fused++
			i += f.consumed
			continue
		}

		it := insts[i]
		newOffset[it.offset] = len(out)
		if it.op == OpJump || it.op == OpJumpNotTrue {
			patches = append(patches, jumpPatch{
				pos:        len(out),
				operandOff: 1,
				target:     it.operands[0],
			})
		}
		out = append(out, ins[it.offset:it.offset+it.size]...)
		appendLine(&outLines, lines, it.offset, len(out)-it.size)
		i++
	}

	if fused == 0 {
		return ins, lines, nil
	}

	for _, p := range patches {
		if err := p.apply(out, newOffset); err != nil {
			return ins, lines, err
		}
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
