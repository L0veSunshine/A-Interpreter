package cfg

// GraphView 是计算支配关系所需的最小图视图。
//
// 抽成接口是为了让"字节码 CFG"和"IR 的 CFG"共用同一份支配关系实现，
// 而不是各写一遍——支配边界算错，phi 就会插在错误的位置，
// 整个 SSA 的正确性都会崩掉，这种算法只应该有一份。
type GraphView interface {
	// NumNodes 返回节点总数（节点用 [0,n) 的整数编号）。
	NumNodes() int
	// EntryNode 返回入口节点编号。
	EntryNode() int
	// PredNodes 返回 v 的前驱编号。
	PredNodes(v int) []int
	// SuccNodes 返回 v 的后继编号。
	SuccNodes(v int) []int
}

// DomTree 是支配关系分析结果。
//
// 采用 Cooper-Harvey-Kennedy 迭代算法求立即支配者，再按定义求支配边界：
// 对每个多前驱节点 b，从每个前驱沿 idom 往上走，走到 idom[b] 之前
// 经过的节点，都以 b 为支配边界。
type DomTree struct {
	n         int
	entry     int
	idom      []int
	rpoNum    []int
	order     []int // 逆后序，支配者一定排在被支配者之前
	frontier  [][]int
	reachable []bool
}

// ComputeDom 计算支配树与支配边界。
func ComputeDom(g GraphView) *DomTree {
	n := g.NumNodes()
	d := &DomTree{
		n:         n,
		entry:     g.EntryNode(),
		idom:      make([]int, n),
		rpoNum:    make([]int, n),
		frontier:  make([][]int, n),
		reachable: make([]bool, n),
	}
	for i := range d.idom {
		d.idom[i] = -1
		d.rpoNum[i] = -1
	}
	if n == 0 || d.entry < 0 || d.entry >= n {
		return d
	}

	d.order = rpoOrder(g)
	for i, v := range d.order {
		d.rpoNum[v] = i
		d.reachable[v] = true
	}
	d.idom[d.entry] = d.entry

	// 迭代到不动点：idom[b] = 所有前驱（已求出 idom 的）在支配树上的最近公共祖先
	for changed := true; changed; {
		changed = false
		for _, b := range d.order {
			if b == d.entry {
				continue
			}
			newIdom := -1
			for _, p := range g.PredNodes(b) {
				if p < 0 || p >= n || !d.reachable[p] || d.idom[p] < 0 {
					continue
				}
				if newIdom < 0 {
					newIdom = p
				} else {
					newIdom = d.intersect(p, newIdom)
				}
			}
			if newIdom >= 0 && d.idom[b] != newIdom {
				d.idom[b] = newIdom
				changed = true
			}
		}
	}

	for _, b := range d.order {
		preds := g.PredNodes(b)
		if len(preds) < 2 {
			continue
		}
		for _, p := range preds {
			if p < 0 || p >= n || !d.reachable[p] || d.idom[p] < 0 {
				continue
			}
			for runner := p; runner >= 0 && runner != d.idom[b]; {
				d.addFrontier(runner, b)
				next := d.idom[runner]
				if next == runner || next < 0 {
					break
				}
				runner = next
			}
		}
	}
	return d
}

// IDom 返回 v 的立即支配者；入口节点返回自身，不可达节点返回 -1。
func (d *DomTree) IDom(v int) int {
	if v < 0 || v >= d.n {
		return -1
	}
	return d.idom[v]
}

// Frontier 返回 v 的支配边界。
func (d *DomTree) Frontier(v int) []int {
	if v < 0 || v >= d.n {
		return nil
	}
	return d.frontier[v]
}

// Dominates 判断 a 是否支配 b。
func (d *DomTree) Dominates(a, b int) bool {
	if a < 0 || b < 0 || a >= d.n || b >= d.n {
		return false
	}
	if !d.reachable[a] || !d.reachable[b] {
		return false
	}
	for cur := b; ; {
		if cur == a {
			return true
		}
		next := d.idom[cur]
		if next < 0 || next == cur {
			return false
		}
		cur = next
	}
}

// Reachable 判断节点是否从入口可达。
func (d *DomTree) Reachable(v int) bool {
	return v >= 0 && v < d.n && d.reachable[v]
}

// Order 返回从入口可达节点的逆后序（支配者在前）。
func (d *DomTree) Order() []int { return d.order }

func (d *DomTree) addFrontier(v, f int) {
	for _, x := range d.frontier[v] {
		if x == f {
			return
		}
	}
	d.frontier[v] = append(d.frontier[v], f)
}

func (d *DomTree) intersect(a, b int) int {
	for a != b {
		for d.rpoNum[a] > d.rpoNum[b] {
			a = d.idom[a]
		}
		for d.rpoNum[b] > d.rpoNum[a] {
			b = d.idom[b]
		}
	}
	return a
}

// rpoOrder 从入口做 DFS 后序再反转，得到逆后序。
func rpoOrder(g GraphView) []int {
	n := g.NumNodes()
	visited := make([]bool, n)
	post := make([]int, 0, n)

	type frame struct {
		v    int
		succ []int
		next int
	}
	entry := g.EntryNode()
	stack := []frame{{v: entry, succ: g.SuccNodes(entry)}}
	visited[entry] = true
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.next < len(top.succ) {
			s := top.succ[top.next]
			top.next++
			if s >= 0 && s < n && !visited[s] {
				visited[s] = true
				stack = append(stack, frame{v: s, succ: g.SuccNodes(s)})
			}
			continue
		}
		post = append(post, top.v)
		stack = stack[:len(stack)-1]
	}

	// 反转
	for i, j := 0, len(post)-1; i < j; i, j = i+1, j-1 {
		post[i], post[j] = post[j], post[i]
	}
	return post
}
