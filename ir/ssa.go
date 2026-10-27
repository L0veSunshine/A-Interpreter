package ir

import (
	"Interpreter/cfg"
)

// 本文件实现经典的 Cytron SSA 构造：
//
//  1. 找出每个被写入的局部槽位，以及在哪些块里被写入；
//  2. 沿支配边界插入 phi；
//  3. 按支配树 DFS 重命名，用"版本栈"把每次读取替换成当前版本。
//
// 为什么只对局部槽位做 SSA：全局变量可能被任意一次调用改写，
// 而本优化器没有跨函数的过程间分析。局部槽位属于当前帧，
// 只有本函数的指令能写它，因此可以安全地做 SSA。
//
// 临时值（栈上的表达式结果）在 build 阶段就已经"单赋值"了，
// 不需要 phi，也不需要重命名。

// localVar 标识一个需要 SSA 的变量。
//
// 编码为 slot*2 + kind：kind=0 是局部槽位，kind=1 是全局槽位。
//
// 全局变量只有在"本函数内没有任何调用"时才参与 SSA：
// 调用可能改写全局，而本优化器没有过程间分析。
// 即便如此，全局的 store 也必须保留（其它函数可能观察它），
// 所以这里只做"读转发"，不做"写消除"。
type localVar int

func localKey(slot int) localVar  { return localVar(slot * 2) }
func globalKey(slot int) localVar { return localVar(slot*2 + 1) }
func varSlot(v localVar) int      { return int(v) / 2 }
func varIsGlobal(v localVar) bool { return int(v)%2 == 1 }

func (f *Function) runSSA() error {
	if f.Entry == nil {
		return nil
	}
	// 全局槽位只在"main 且没有任何调用"时才当私有变量处理：
	// 此时没有别的代码能观察到它们，可以彻底消除 store。
	// 其它函数一律不碰全局——调用可能改写任意全局。
	f.ssaGlobals = f.isMain && !f.hasCall
	f.dropGlobalStores = f.ssaGlobals

	dom := f.computeDom()

	defs := f.collectVars()
	liveIn := f.localLiveIn()
	if f.ssaGlobals {
		// 只把"不需要插 phi"的全局当作私有变量：
		// 循环里的全局一旦需要 phi，就会多出一堆拷贝，反而不如原地读写省。
		f.filterGlobalsNeedingPhi(dom, defs, liveIn)
	}
	f.insertPhis(dom, defs, liveIn)
	if err := f.rename(dom); err != nil {
		return err
	}
	f.removeLocalAccesses()
	return nil
}

// filterGlobalsNeedingPhi 从 defs 里剔除"需要 phi"的全局变量，
// 并把它们记到 noSSAGlobals 里，后续不参与重命名。
func (f *Function) filterGlobalsNeedingPhi(dom *cfg.DomTree,
	defs map[localVar]map[*Block]bool, liveIn map[localVar]map[*Block]bool) {
	for v, blocks := range defs {
		if !varIsGlobal(v) {
			continue
		}
		needPhi := false
		for b := range blocks {
			for _, fid := range dom.Frontier(b.ID) {
				if liveIn[v][f.Blocks[fid]] {
					needPhi = true
				}
			}
		}
		if !needPhi {
			continue
		}
		delete(defs, v)
		if f.noSSAGlobals == nil {
			f.noSSAGlobals = map[int]bool{}
		}
		f.noSSAGlobals[varSlot(v)] = true
	}
}

// computeDom 在 IR 上计算支配关系，复用 cfg 的通用实现。
func (f *Function) computeDom() *cfg.DomTree {
	return cfg.ComputeDom(&funcGraph{fn: f})
}

// funcGraph 把 IR 函数适配成 cfg.GraphView。
type funcGraph struct{ fn *Function }

func (g *funcGraph) NumNodes() int { return len(g.fn.Blocks) }

func (g *funcGraph) EntryNode() int {
	if g.fn.Entry == nil {
		return -1
	}
	return g.fn.Entry.ID
}

func (g *funcGraph) PredNodes(v int) []int {
	preds := g.fn.Blocks[v].Preds
	out := make([]int, len(preds))
	for i, p := range preds {
		out[i] = p.ID
	}
	return out
}

func (g *funcGraph) SuccNodes(v int) []int {
	succs := g.fn.Blocks[v].Succs
	out := make([]int, len(succs))
	for i, s := range succs {
		out[i] = s.ID
	}
	return out
}

// collectVars 返回"变量 -> 定义它的块集合"。
func (f *Function) collectVars() map[localVar]map[*Block]bool {
	defs := map[localVar]map[*Block]bool{}
	mark := func(v localVar, b *Block) {
		if defs[v] == nil {
			defs[v] = map[*Block]bool{}
		}
		defs[v][b] = true
	}
	for _, b := range f.Blocks {
		for _, in := range b.Instrs {
			switch in.Op {
			case OpInit, OpStoreLocal:
				mark(localKey(in.Imm), b)
			case OpStoreGlobal:
				if f.ssaGlobals {
					mark(globalKey(in.Imm), b)
				}
			}
		}
	}
	return defs
}

// localLiveIn 计算"每个槽位在哪些块的入口处仍然活跃"。
// 用于剪枝 phi：只在真正需要的地方插 phi（pruned SSA），
// 否则会给在某条路径上根本没定义过的槽位插 phi，实参无从填充。
func (f *Function) localLiveIn() map[localVar]map[*Block]bool {
	use := map[localVar]map[*Block]bool{}
	def := map[localVar]map[*Block]bool{}
	mark := func(m map[localVar]map[*Block]bool, v localVar, b *Block) {
		if m[v] == nil {
			m[v] = map[*Block]bool{}
		}
		m[v][b] = true
	}

	for _, b := range f.Blocks {
		defined := map[localVar]bool{}
		for _, in := range b.Instrs {
			switch in.Op {
			case OpInit, OpStoreLocal:
				v := localKey(in.Imm)
				mark(def, v, b)
				defined[v] = true
			case OpStoreGlobal:
				if f.ssaGlobals {
					v := globalKey(in.Imm)
					mark(def, v, b)
					defined[v] = true
				}
			case OpLoadLocal:
				v := localKey(in.Imm)
				if !defined[v] {
					mark(use, v, b)
				}
			case OpLoadGlobal:
				if f.ssaGlobals {
					v := globalKey(in.Imm)
					if !defined[v] {
						mark(use, v, b)
					}
				}
			}
		}
	}

	liveIn := map[localVar]map[*Block]bool{}
	for v := range use {
		liveIn[v] = map[*Block]bool{}
	}
	for v := range def {
		if liveIn[v] == nil {
			liveIn[v] = map[*Block]bool{}
		}
	}

	order := f.reversePostOrder()
	for changed := true; changed; {
		changed = false
		for i := len(order) - 1; i >= 0; i-- {
			b := order[i]

			out := map[localVar]bool{}
			for _, s := range b.Succs {
				for v, blocks := range liveIn {
					if blocks[s] {
						out[v] = true
					}
				}
			}

			in := map[localVar]bool{}
			for v := range out {
				if !def[v][b] {
					in[v] = true
				}
			}
			for v := range use {
				if use[v][b] {
					in[v] = true
				}
			}

			cur := map[localVar]bool{}
			for v, blocks := range liveIn {
				if blocks[b] {
					cur[v] = true
				}
			}
			if sameLocalSet(in, cur) {
				continue
			}
			for v := range liveIn {
				if in[v] {
					liveIn[v][b] = true
				} else {
					delete(liveIn[v], b)
				}
			}
			changed = true
		}
	}
	return liveIn
}

func sameLocalSet(a, b map[localVar]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// insertPhis 沿支配边界插入 phi（剪枝版：只在槽位活跃的块插）。
//
// 标准算法：把定义了 v 的块放进工作队列；对每个块 b 的每个支配边界 f，
// 若 f 还没有 v 的 phi 就插一个；若 v 在 f 里没有被定义过，则把 f 也入队。
func (f *Function) insertPhis(dom *cfg.DomTree, defs map[localVar]map[*Block]bool,
	liveIn map[localVar]map[*Block]bool) {
	vars := make([]localVar, 0, len(defs))
	for v := range defs {
		vars = append(vars, v)
	}
	sortLocalVars(vars)

	for _, v := range vars {
		definedIn := defs[v]
		work := make([]*Block, 0, len(definedIn))
		for b := range definedIn {
			work = append(work, b)
		}
		sortBlocksByID(work)

		hasPhi := map[*Block]bool{}
		for len(work) > 0 {
			b := work[len(work)-1]
			work = work[:len(work)-1]
			for _, fid := range dom.Frontier(b.ID) {
				target := f.Blocks[fid]
				if hasPhi[target] {
					continue
				}
				// 剪枝：该槽位在这里根本不活跃就不需要 phi
				if liveIn != nil && !liveIn[v][target] {
					continue
				}
				hasPhi[target] = true
				phi := &Instr{Op: OpPhi, Imm: int(v), Dst: f.NewValue()}
				// phi 必须位于块首（有形指令之前）
				target.Instrs = append([]*Instr{phi}, target.Instrs...)
				f.defs[phi.Dst] = phi
				if !definedIn[target] {
					work = append(work, target)
				}
			}
		}
	}
}

// rename 按支配树 DFS 重命名，并补全 phi 的实参。
func (f *Function) rename(dom *cfg.DomTree) error {
	stacks := map[localVar][]ValueID{}

	var walk func(b *Block) error
	walk = func(b *Block) error {
		pushed := map[localVar]int{}
		// 块内替换表：OpLoadLocal 的结果被"当前版本"取代
		replace := map[ValueID]ValueID{}

		for _, in := range b.Instrs {
			// 先把实参解析成最终版本（同一块内先前 load 产生的别名）
			for i, arg := range in.Args {
				in.Args[i] = resolveValue(arg, replace)
			}

			switch in.Op {
			case OpPhi:
				v := varOf(in)
				stacks[v] = append(stacks[v], in.Dst)
				pushed[v]++
			case OpInit:
				v := localKey(in.Imm)
				stacks[v] = append(stacks[v], in.Dst)
				pushed[v]++
			case OpLoadLocal:
				v := localKey(in.Imm)
				cur, ok := topValue(stacks[v])
				if !ok {
					return bail("read of local slot %d before any definition (block %d)", in.Imm, b.ID)
				}
				replace[in.Dst] = cur
				in.Op = OpNop
			case OpStoreLocal:
				v := localKey(in.Imm)
				if len(in.Args) != 1 {
					return bail("malformed store.local")
				}
				stacks[v] = append(stacks[v], in.Args[0])
				pushed[v]++
				in.Op = OpNop
			case OpLoadGlobal:
				if !f.ssaGlobals || f.noSSAGlobals[in.Imm] {
					continue
				}
				v := globalKey(in.Imm)
				cur, ok := topValue(stacks[v])
				if !ok {
					return bail("read of global %d before any definition in %s", in.Imm, f.Name)
				}
				replace[in.Dst] = cur
				in.Op = OpNop
			case OpStoreGlobal:
				if !f.ssaGlobals || f.noSSAGlobals[in.Imm] {
					continue
				}
				v := globalKey(in.Imm)
				if len(in.Args) != 1 {
					return bail("malformed store.global")
				}
				stacks[v] = append(stacks[v], in.Args[0])
				pushed[v]++
				if f.dropGlobalStores {
					// main 无调用：全局是私有的，写操作可以彻底消掉
					in.Op = OpNop
				}
			}
		}

		// 本块出口处各槽位的版本，供后继块的 phi 填实参
		exitVersion := make(map[localVar]ValueID, len(stacks))
		for v, st := range stacks {
			if len(st) > 0 {
				exitVersion[v] = st[len(st)-1]
			}
		}

		for _, s := range b.Succs {
			idx := predIndex(s, b)
			if idx < 0 {
				return bail("internal: predecessor index not found")
			}
			for _, in := range s.Instrs {
				if in.Op != OpPhi {
					continue
				}
				v := varOf(in)
				cur, ok := exitVersion[v]
				if !ok {
					return bail("phi for local slot %d at block %d has no incoming value", in.Imm, s.ID)
				}
				for len(in.Args) <= idx {
					in.Args = append(in.Args, NoValue)
				}
				in.Args[idx] = cur
			}
		}

		// 递归处理支配树上的子节点
		for _, child := range f.Blocks {
			if child == b {
				continue
			}
			if dom.IDom(child.ID) != b.ID {
				continue
			}
			if err := walk(child); err != nil {
				return err
			}
		}

		for v, n := range pushed {
			st := stacks[v]
			if n > len(st) {
				n = len(st)
			}
			stacks[v] = st[:len(st)-n]
		}
		return nil
	}

	if err := walk(f.Entry); err != nil {
		return err
	}
	// phi 实参必须全部填满
	for _, b := range f.Blocks {
		for _, in := range b.Instrs {
			if in.Op != OpPhi {
				continue
			}
			if len(in.Args) != len(b.Preds) {
				return bail("phi at block %d has %d operands but %d predecessors",
					b.ID, len(in.Args), len(b.Preds))
			}
			for _, a := range in.Args {
				if a == NoValue {
					return bail("phi at block %d has an undefined operand", b.ID)
				}
			}
		}
	}
	return nil
}

func resolveValue(v ValueID, replace map[ValueID]ValueID) ValueID {
	if len(replace) == 0 {
		return v
	}
	for i := 0; i < 1<<16; i++ {
		next, ok := replace[v]
		if !ok {
			return v
		}
		v = next
	}
	return v
}

func predIndex(b, pred *Block) int {
	for i, p := range b.Preds {
		if p == pred {
			return i
		}
	}
	return -1
}

// varOf 返回 phi 所定义的变量。栈上的 phi（本实现不使用）会返回一个
// 负数键，从而不会与真实变量混淆。
func varOf(in *Instr) localVar {
	if in.Imm < 0 {
		return localVar(-1)
	}
	return localVar(in.Imm)
}

func topValue(st []ValueID) (ValueID, bool) {
	if len(st) == 0 {
		return NoValue, false
	}
	return st[len(st)-1], true
}

// removeLocalAccesses 删除 SSA 过程中被标记为 Nop 的指令。
//
// 只删 OpNop：rename 只会把真正被消解（换成版本引用）的 load/store
// 标成 Nop；未参与 SSA 的全局读写仍然是正常指令，必须保留。
func (f *Function) removeLocalAccesses() {
	for _, b := range f.Blocks {
		out := b.Instrs[:0]
		for _, in := range b.Instrs {
			if in.Op == OpNop {
				continue
			}
			out = append(out, in)
		}
		b.Instrs = out
	}
}

func sortLocalVars(vs []localVar) {
	for i := 1; i < len(vs); i++ {
		for j := i; j > 0 && vs[j] < vs[j-1]; j-- {
			vs[j], vs[j-1] = vs[j-1], vs[j]
		}
	}
}

func sortBlocksByID(bs []*Block) {
	for i := 1; i < len(bs); i++ {
		for j := i; j > 0 && bs[j].ID < bs[j-1].ID; j-- {
			bs[j], bs[j-1] = bs[j-1], bs[j]
		}
	}
}
