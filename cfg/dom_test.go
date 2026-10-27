package cfg_test

import (
	. "Interpreter/cfg"
	"testing"
)

// blockByLastOp 找到以某种跳转结束的块，便于在测试里定位结构。
func blocksEndingWith(g *Graph, pred func(Op) bool) []*Block {
	var res []*Block
	for _, b := range g.Blocks {
		if last, ok := b.Last(); ok && pred(last) {
			res = append(res, b)
		}
	}
	return res
}

func TestDominatorsDiamond(t *testing.T) {
	b := compileSrc(t, `var x=0
if (true){
	x=1
}else{
	x=2
}
x`)
	g := mustBuild(t, b)
	d := g.Dominators()

	if !d.Dominates(g.Entry, g.Blocks[len(g.Blocks)-1]) {
		t.Fatal("entry must dominate every reachable block")
	}

	// if 的两个分支互不支配，它们共同的支配边界应当是汇合块
	cond := blocksEndingWith(g, func(o Op) bool { return o.IsCondJump() })
	if len(cond) != 1 {
		t.Fatalf("expect exactly one conditional jump, got %d", len(cond))
	}
	succs := cond[0].Succ
	if len(succs) != 2 {
		t.Fatalf("conditional jump should have 2 successors, got %d", len(succs))
	}
	for _, br := range succs {
		merged := false
		for _, f := range d.Frontier(br) {
			if f == succs[0] || f == succs[1] {
				continue
			}
			merged = true
			if !d.Dominates(cond[0], f) {
				t.Fatalf("dominance frontier must be dominated by the branch head")
			}
		}
		if !merged {
			t.Fatalf("block %d (branch) should have a non-sibling dominance frontier", br.ID)
		}
	}
}

func TestDominatorsLoopHeader(t *testing.T) {
	b := compileSrc(t, `var s=0
for(var i=0;i<10;i=i+1){
	s=s+i
}
s`)
	g := mustBuild(t, b)
	d := g.Dominators()

	var header *Block
	for _, blk := range g.Blocks {
		for _, s := range blk.Succ {
			if s.Start <= blk.Start {
				header = s // 回边的目标就是循环头
			}
		}
	}
	if header == nil {
		t.Fatal("loop header not found")
	}

	// 循环体内定义的值，支配边界应该包含循环头（这里要插 phi）
	found := false
	for _, blk := range g.Blocks {
		if blk.Start <= header.Start {
			continue
		}
		for _, f := range d.Frontier(blk) {
			if f == header {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("a block inside the loop must have the loop header in its dominance frontier")
	}
}

func TestDominatorsStraightLine(t *testing.T) {
	b := compileSrc(t, `var a=1
a=a+1
a`)
	g := mustBuild(t, b)
	d := g.Dominators()
	if len(g.Blocks) != 1 {
		t.Fatalf("straight-line program should be one block, got %d", len(g.Blocks))
	}
	if got := d.Frontier(g.Entry); len(got) != 0 {
		t.Fatalf("single block has empty frontier, got %d", len(got))
	}
	if d.IDom(g.Entry) != g.Entry {
		t.Fatal("entry's immediate dominator is itself")
	}
}
