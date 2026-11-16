package ir

// 本文件做两件事：
//
//  1. 值级活跃性分析（liveness）——SSA 之后"变量"已经变成"值"，
//     活跃性直接在值上算，比在槽位上算更精确。
//  2. 槽位分配（寄存器分配的简化版）：冲突图 + 贪心着色。
//
// 分配出的槽位就是 VM 帧里的局部槽位数组下标。
// OpInit（形参）必须固定在原槽位上，因为 callFunc 按参数序号写入槽位。

// computeLiveness 迭代到不动点求每块的 liveIn / liveOut。
func (f *Function) computeLiveness() {
	for _, b := range f.Blocks {
		b.liveIn = map[ValueID]bool{}
		b.liveOut = map[ValueID]bool{}
	}
	// 逆后序便于快速收敛
	order := f.reversePostOrder()

	for changed := true; changed; {
		changed = false
		for i := len(order) - 1; i >= 0; i-- {
			b := order[i]
			out := map[ValueID]bool{}
			for _, s := range b.Succs {
				idx := predIndex(s, b)
				phiDsts := map[ValueID]bool{}
				for _, in := range s.Instrs {
					if in.Op != OpPhi {
						continue
					}
					phiDsts[in.Dst] = true
					if idx >= 0 && idx < len(in.Args) && in.Args[idx] != NoValue {
						out[in.Args[idx]] = true
					}
				}
				for v := range s.liveIn {
					if !phiDsts[v] {
						out[v] = true
					}
				}
			}
			if !sameSet(out, b.liveOut) {
				b.liveOut = out
				changed = true
			}

			in := map[ValueID]bool{}
			for v := range out {
				in[v] = true
			}
			// 反向扫块内指令：先删 def，再加 use
			for j := len(b.Instrs) - 1; j >= 0; j-- {
				inst := b.Instrs[j]
				if inst.Op == OpPhi {
					continue // phi 的实参属于前驱的 liveOut
				}
				if inst.Dst != NoValue {
					delete(in, inst.Dst)
				}
				if inst.Dst2 != NoValue {
					delete(in, inst.Dst2)
				}
				for _, a := range inst.Args {
					if a != NoValue {
						in[a] = true
					}
				}
			}
			if !sameSet(in, b.liveIn) {
				b.liveIn = in
				changed = true
			}
		}
	}
}

func sameSet(a, b map[ValueID]bool) bool {
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

// reversePostOrder 返回从入口可达块的逆后序。
func (f *Function) reversePostOrder() []*Block {
	visited := map[*Block]bool{}
	var post []*Block
	var walk func(b *Block)
	walk = func(b *Block) {
		visited[b] = true
		for _, s := range b.Succs {
			if !visited[s] {
				walk(s)
			}
		}
		post = append(post, b)
	}
	if f.Entry != nil {
		walk(f.Entry)
	}
	for i, j := 0, len(post)-1; i < j; i, j = i+1, j-1 {
		post[i], post[j] = post[j], post[i]
	}
	return post
}

// allocateSlots 做冲突图着色，返回需要的槽位总数。
//
// 冲突边来源：
//   - 同一块内，一个值的定义处与它之后仍然活跃的其它值冲突；
//   - phi 的目标与每个实参冲突（它们在边上是同时读到的）；
//   - 同一个前驱边上，所有 phi 实参之间互相冲突——否则并行拷贝会串值。
func (f *Function) allocateSlots() int {
	f.computeLiveness()

	adj := make([]map[ValueID]bool, int(f.numValue)+1)
	addEdge := func(a, b ValueID) {
		if a == NoValue || b == NoValue || a == b {
			return
		}
		if adj[a] == nil {
			adj[a] = map[ValueID]bool{}
		}
		if adj[b] == nil {
			adj[b] = map[ValueID]bool{}
		}
		adj[a][b] = true
		adj[b][a] = true
	}

	for _, b := range f.Blocks {
		live := map[ValueID]bool{}
		for v := range b.liveOut {
			live[v] = true
		}
		for i := len(b.Instrs) - 1; i >= 0; i-- {
			in := b.Instrs[i]
			// phi 的目标在块入口处就活跃，因此也要和 liveOut 里的值冲突。
			// 注意这里**不**加"phi 目标 vs 实参"的边：降级时边上的拷贝是
			// 并行拷贝（先全部求值、再全部写回），实参与目标共用槽位是安全的，
			// 而且正是靠它才能把循环携带变量的拷贝合并掉。
			for _, dst := range []ValueID{in.Dst, in.Dst2} {
				if dst == NoValue {
					continue
				}
				for v := range live {
					addEdge(dst, v)
				}
				delete(live, dst)
			}
			if in.Dst != NoValue && in.Dst2 != NoValue {
				addEdge(in.Dst, in.Dst2)
			}
			if in.Op == OpPhi {
				// phi 的实参属于前驱块，不算本块的活跃值
				continue
			}
			for _, a := range in.Args {
				if a != NoValue {
					live[a] = true
				}
			}
		}

		// 同一前驱边上的 phi 实参互斥，避免并行拷贝串值
		for _, s := range b.Succs {
			idx := predIndex(s, b)
			if idx < 0 {
				continue
			}
			var args []ValueID
			for _, in := range s.Instrs {
				if in.Op == OpPhi && idx < len(in.Args) {
					args = append(args, in.Args[idx])
				}
			}
			for i := 0; i < len(args); i++ {
				for j := i + 1; j < len(args); j++ {
					addEdge(args[i], args[j])
				}
			}
		}
	}

	f.slot = make([]int, int(f.numValue)+1)
	for i := range f.slot {
		f.slot[i] = -1
	}

	// 合并偏好（coalescing）：phi 实参优先和 phi 目标共用槽位。
	// 这样边上那条拷贝就是恒等拷贝，降级时可以直接省掉——
	// 循环携带变量（每次迭代都要搬运）靠它才不会被"存-取"多付一次。
	// 安全性由干涉图保证：真同时活跃时 taken 里已经占了目标槽位。
	prefer := make([]ValueID, int(f.numValue)+1)
	for _, b := range f.Blocks {
		if len(b.Succs) != 1 {
			continue
		}
		s := b.Succs[0]
		idx := predIndex(s, b)
		if idx < 0 {
			continue
		}
		for _, in := range s.Instrs {
			if in.Op != OpPhi || idx >= len(in.Args) {
				continue
			}
			a := in.Args[idx]
			if a != NoValue && a != in.Dst && prefer[a] == NoValue {
				prefer[a] = in.Dst
			}
		}
	}

	// 形参固定原槽位
	used := map[int]bool{}
	for v := ValueID(1); int(v) <= int(f.numValue); v++ {
		d := f.Def(v)
		if d != nil && d.Op == OpInit {
			f.slot[v] = d.Imm
			used[d.Imm] = true
		}
	}

	// 贪心着色：定义越晚的值越先分配（近似按活跃区间末尾排序）。
	// 常量不参与分配——它们在降级时会被重新发射成 OpConstant。
	order := make([]ValueID, 0, int(f.numValue))
	for v := ValueID(1); int(v) <= int(f.numValue); v++ {
		if f.slot[v] >= 0 {
			continue
		}
		if d := f.Def(v); d != nil && d.Op == OpConst {
			continue
		}
		order = append(order, v)
	}
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			if order[j] > order[i] {
				order[i], order[j] = order[j], order[i]
			}
		}
	}

	maxSlot := -1
	for s := range used {
		if s > maxSlot {
			maxSlot = s
		}
	}
	for _, v := range order {
		taken := map[int]bool{}
		for n := range adj[v] {
			if f.slot[n] >= 0 {
				taken[f.slot[n]] = true
			}
		}
		s := -1
		if p := prefer[v]; p != NoValue && f.slot[p] >= 0 && !taken[f.slot[p]] {
			s = f.slot[p]
		}
		if s < 0 {
			s = 0
			for taken[s] {
				s++
			}
		}
		f.slot[v] = s
		if s > maxSlot {
			maxSlot = s
		}
	}
	return maxSlot + 1
}
