package cfg

// DomInfo 是字节码 CFG 上的支配关系视图，内部复用 ComputeDom。
//
// 保留 *Block 风格的 API 是为了让调用方（以及测试）读起来自然；
// 真正的算法实现只有 cfg/graph.go 里那一份。
type DomInfo struct {
	tree  *DomTree
	graph *Graph
}

// IDom 返回 b 的立即支配者；入口块的立即支配者是它自己。
func (d *DomInfo) IDom(b *Block) *Block {
	if d.tree == nil || b == nil {
		return nil
	}
	id := d.tree.IDom(b.ID)
	if id < 0 || id >= len(d.graph.Blocks) {
		return nil
	}
	return d.graph.Blocks[id]
}

// Frontier 返回 b 的支配边界（需要在这里为 b 中定义的值插 phi 的块）。
func (d *DomInfo) Frontier(b *Block) []*Block {
	if d.tree == nil || b == nil {
		return nil
	}
	ids := d.tree.Frontier(b.ID)
	out := make([]*Block, 0, len(ids))
	for _, id := range ids {
		if id >= 0 && id < len(d.graph.Blocks) {
			out = append(out, d.graph.Blocks[id])
		}
	}
	return out
}

// Dominates 判断 a 是否支配 b。
func (d *DomInfo) Dominates(a, b *Block) bool {
	if d.tree == nil || a == nil || b == nil {
		return false
	}
	return d.tree.Dominates(a.ID, b.ID)
}

// Dominators 计算该 CFG 的支配关系。
func (g *Graph) Dominators() *DomInfo {
	return &DomInfo{tree: ComputeDom(blockGraph{g}), graph: g}
}

// blockGraph 把 *Graph 适配成 GraphView，节点编号就是 Block.ID
// （Build 保证 Block.ID 与其在 Blocks 切片中的下标一致）。
type blockGraph struct{ g *Graph }

func (bg blockGraph) NumNodes() int { return len(bg.g.Blocks) }

func (bg blockGraph) EntryNode() int {
	if bg.g.Entry == nil {
		return -1
	}
	return bg.g.Entry.ID
}

func (bg blockGraph) PredNodes(v int) []int {
	out := make([]int, len(bg.g.Blocks[v].Pred))
	for i, p := range bg.g.Blocks[v].Pred {
		out[i] = p.ID
	}
	return out
}

func (bg blockGraph) SuccNodes(v int) []int {
	out := make([]int, len(bg.g.Blocks[v].Succ))
	for i, s := range bg.g.Blocks[v].Succ {
		out[i] = s.ID
	}
	return out
}
