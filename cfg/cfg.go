package cfg

import (
	"Interpreter/code"
	"fmt"
	"sort"
	"strings"
)

// Op 是一条已解码的指令，Offset 是它在指令流里的字节偏移。
type Op struct {
	Offset   int
	Opcode   code.Opcode
	Operands []int
	Width    int // 含操作码在内的总字节数
}

func (o Op) Name() string {
	return code.Definitions[o.Opcode].Name
}

// IsJump 判断是否为无条件跳转。
func (o Op) IsJump() bool { return o.Opcode == code.OpJump }

// IsCondJump 判断是否为条件跳转。
func (o Op) IsCondJump() bool { return o.Opcode == code.OpJumpNotTrue }

// IsExit 判断该指令是否结束当前函数的控制流。
func (o Op) IsExit() bool { return o.Opcode == code.OpReturnVal }

// Block 是基本块：一段只有单入口单出口的直线指令序列。
type Block struct {
	ID        int
	Start     int // 首指令偏移
	End       int // 结束偏移（不含）
	Ops       []Op
	Succ      []*Block
	Pred      []*Block
	Reachable bool
}

func (b *Block) Last() (Op, bool) {
	if len(b.Ops) == 0 {
		return Op{}, false
	}
	return b.Ops[len(b.Ops)-1], true
}

// Graph 是一个函数的控制流图。
type Graph struct {
	Entry    *Block
	Blocks   []*Block
	byOffset map[int]*Block
}

// Build 把线性指令流切成基本块并连边。
// 切分依据是"首指令"(leader)：第 0 条指令、任何跳转目标、
// 任何跳转/返回之后的那条指令。leader 之间的指令序列天然就是一个基本块，
// 也就是说"把连续语句合并成块"这一步由 leader 划分直接得到。
func Build(ins code.Instructions) (*Graph, error) {
	ops, err := Decode(ins)
	if err != nil {
		return nil, err
	}
	g := &Graph{byOffset: map[int]*Block{}}
	if len(ops) == 0 {
		return g, nil
	}

	leaders := map[int]bool{0: true}
	for _, o := range ops {
		switch {
		case o.IsJump(), o.IsCondJump():
			leaders[o.Operands[0]] = true
			leaders[o.Offset+o.Width] = true
		case o.IsExit():
			leaders[o.Offset+o.Width] = true
		}
	}
	// 只保留落在指令流内的 leader
	for start := range leaders {
		if start < 0 || start >= len(ins) {
			delete(leaders, start)
		}
	}

	var cur *Block
	for _, o := range ops {
		if leaders[o.Offset] || cur == nil {
			cur = &Block{ID: len(g.Blocks), Start: o.Offset}
			g.Blocks = append(g.Blocks, cur)
			g.byOffset[o.Offset] = cur
		}
		cur.Ops = append(cur.Ops, o)
		cur.End = o.Offset + o.Width
	}

	for _, b := range g.Blocks {
		last, ok := b.Last()
		if !ok {
			continue
		}
		switch {
		case last.IsJump():
			g.link(b, last.Operands[0])
		case last.IsCondJump():
			g.link(b, last.Operands[0])
			g.link(b, b.End) // 条件不成立时顺序落到下一块
		case last.IsExit():
		default:
			g.link(b, b.End)
		}
	}
	g.Entry = g.byOffset[0]
	g.markReachable()
	return g, nil
}

func (g *Graph) link(from *Block, targetOffset int) {
	to, ok := g.byOffset[targetOffset]
	if !ok {
		return // 跳到指令流末尾，等于函数结束
	}
	for _, s := range from.Succ {
		if s == to {
			return
		}
	}
	from.Succ = append(from.Succ, to)
	to.Pred = append(to.Pred, from)
}

func (g *Graph) markReachable() {
	for _, b := range g.Blocks {
		b.Reachable = false
	}
	if g.Entry == nil {
		return
	}
	queue := []*Block{g.Entry}
	g.Entry.Reachable = true
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, s := range cur.Succ {
			if !s.Reachable {
				s.Reachable = true
				queue = append(queue, s)
			}
		}
	}
}

// Unreachable 返回所有不可达基本块（死代码）。
func (g *Graph) Unreachable() []*Block {
	var res []*Block
	for _, b := range g.Blocks {
		if !b.Reachable {
			res = append(res, b)
		}
	}
	return res
}

// HasBackEdge 判断是否存在回边（即循环）。
func (g *Graph) HasBackEdge() bool {
	for _, b := range g.Blocks {
		for _, s := range b.Succ {
			if s.Start <= b.Start {
				return true
			}
		}
	}
	return false
}

// Compact 做三件事：
//  1. 丢掉不可达的基本块（不可达语句是死代码）；
//  2. 丢掉"跳到紧邻下一块"的无条件跳转 —— 这正是 if 没有 else 时
//     编译器多生成的那条 OpJump，去掉后两块自然合并成一个基本块；
//  3. 重算所有跳转目标偏移。
//
// 返回重排后的新指令流；调用方需要重新 Build 才能拿到新图。
// Compact 删除不可达块与"跳到紧邻下一块"的冗余跳转。
//
// 注意：压缩会改变指令偏移，因此**行号表必须一起重映射**，
// 否则运行时报错会指到错误的源码行。需要行号请用 CompactLines。
func (g *Graph) Compact(ins code.Instructions) (code.Instructions, error) {
	out, _, err := g.CompactLines(ins, nil)
	return out, err
}

// CompactLines 与 Compact 相同，但额外把行号表重映射到新偏移上。
func (g *Graph) CompactLines(ins code.Instructions, lines []code.LineEntry) (code.Instructions, []code.LineEntry, error) {
	live := make([]*Block, 0, len(g.Blocks))
	for _, b := range g.Blocks {
		if b.Reachable {
			live = append(live, b)
		}
	}
	sort.Slice(live, func(i, j int) bool { return live[i].Start < live[j].Start })

	// 记录哪些基本块末尾那条跳转是"跳到紧邻下一块"，可以整条删掉
	dropLast := make(map[*Block]bool)
	for i, b := range live {
		last, ok := b.Last()
		if !ok || !last.IsJump() || i+1 >= len(live) {
			continue
		}
		if last.Operands[0] == live[i+1].Start {
			dropLast[b] = true
		}
	}

	// 第一遍：算出每个基本块在新指令流里的起始偏移
	newStart := make(map[*Block]int, len(live))
	pos := 0
	for _, b := range live {
		newStart[b] = pos
		for i, o := range b.Ops {
			if dropLast[b] && i == len(b.Ops)-1 {
				continue
			}
			pos += o.Width
		}
	}

	// 第二遍：按需改写跳转操作数并输出
	out := make(code.Instructions, 0, pos)
	// oldOffset -> newOffset，用于重映射行号表
	moved := make(map[int]int, len(ins))
	for _, b := range live {
		for i, o := range b.Ops {
			if dropLast[b] && i == len(b.Ops)-1 {
				continue
			}
			moved[o.Offset] = len(out)
			operands := o.Operands
			if o.IsJump() || o.IsCondJump() {
				to, ok := g.byOffset[o.Operands[0]]
				if !ok {
					return nil, nil, fmt.Errorf("cfg: jump at %d targets outside instructions", o.Offset)
				}
				if !to.Reachable {
					return nil, nil, fmt.Errorf("cfg: reachable jump at %d targets dead block", o.Offset)
				}
				operands = []int{newStart[to]}
			}
			out = append(out, code.Make(o.Opcode, operands...)...)
		}
	}
	if len(out) != pos {
		return nil, nil, fmt.Errorf("cfg: layout mismatch: %d != %d", len(out), pos)
	}
	return out, remapLines(lines, moved), nil
}

// remapLines 把行号表按"旧偏移 -> 新偏移"重映射。
// 找不到对应位置的条目直接丢弃（它属于被删掉的指令）。
func remapLines(lines []code.LineEntry, moved map[int]int) []code.LineEntry {
	if len(lines) == 0 {
		return lines
	}
	out := make([]code.LineEntry, 0, len(lines))
	for _, e := range lines {
		to, ok := moved[e.Offset]
		if !ok {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Line == e.Line {
			continue
		}
		out = append(out, code.LineEntry{Offset: to, Line: e.Line})
	}
	return out
}

// Decode 把字节流解码成指令列表。
func Decode(ins code.Instructions) ([]Op, error) {
	var ops []Op
	for i := 0; i < len(ins); {
		opcode := code.Opcode(ins[i])
		def, ok := code.Definitions[opcode]
		if !ok {
			return nil, fmt.Errorf("cfg: unknown opcode %d at %d", opcode, i)
		}
		operands, w := code.ReadOperand(def, ins[i+1:])
		ops = append(ops, Op{Offset: i, Opcode: opcode, Operands: operands, Width: 1 + w})
		i += 1 + w
	}
	return ops, nil
}

// String 以文本形式打印控制流图。
func (g *Graph) String() string {
	var sb strings.Builder
	for _, b := range g.Blocks {
		mark := ""
		if !b.Reachable {
			mark = " (unreachable)"
		}
		fmt.Fprintf(&sb, "block %d [%d,%d)%s\n", b.ID, b.Start, b.End, mark)
		for _, o := range b.Ops {
			args := ""
			if len(o.Operands) > 0 {
				parts := make([]string, len(o.Operands))
				for i, v := range o.Operands {
					parts[i] = fmt.Sprint(v)
				}
				args = " " + strings.Join(parts, " ")
			}
			fmt.Fprintf(&sb, "  %4d  %-16s%s\n", o.Offset, o.Name(), args)
		}
		preds := make([]string, len(b.Pred))
		for i, p := range b.Pred {
			preds[i] = fmt.Sprint(p.ID)
		}
		succs := make([]string, len(b.Succ))
		for i, s := range b.Succ {
			succs[i] = fmt.Sprint(s.ID)
		}
		fmt.Fprintf(&sb, "        pred=[%s] succ=[%s]\n",
			strings.Join(preds, ","), strings.Join(succs, ","))
	}
	return sb.String()
}
