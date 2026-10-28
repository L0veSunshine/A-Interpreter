package ir

import "fmt"

// 本文件维护 IR 层面的两个不变式：
//
//  1. 每个块的 Preds/Succs 与实际 terminator 一致；
//  2. 每个 phi 的实参个数等于所在块的前驱个数，且第 i 个实参对应第 i 个前驱。
//
// 第 2 条很容易被破坏：任何"把 OpBranch 改成 OpJump"的优化都会让某个前驱
// 消失，而 phi 的实参是按旧前驱顺序排的。一旦不同步，emitPhiCopies 用
// predIndex 取到的实参就会错位，程序会算出**错误结果**（不是崩溃）。
//
// 所以约定：任何修改 terminator 的 pass 结束时都必须调用 relink()。

// relink 重算 CFG 的边，并按新的前驱顺序重建 phi 实参。
//
// 实参按"前驱块指针"搬运，因此即使块被删除或顺序变化也不会错位；
// 消失的前驱对应的实参被丢弃。
func (f *Function) relink() error {
	oldPreds := make(map[*Block][]*Block, len(f.Blocks))
	oldArgs := make(map[*Instr][]ValueID)
	for _, b := range f.Blocks {
		oldPreds[b] = append([]*Block(nil), b.Preds...)
		for _, in := range b.Instrs {
			if in.Op == OpPhi {
				oldArgs[in] = append([]ValueID(nil), in.Args...)
			}
		}
	}

	f.linkBlocks()

	for _, b := range f.Blocks {
		preds, ok := oldPreds[b]
		if !ok {
			// 新建的块（例如临界边拆出来的）不会有 phi
			continue
		}
		for _, in := range b.Instrs {
			if in.Op != OpPhi {
				continue
			}
			args, ok := oldArgs[in]
			if !ok {
				return bail("phi without recorded arguments in block %d", b.ID)
			}
			if len(args) != len(preds) {
				return bail("phi in block %d has %d args but %d old preds",
					b.ID, len(args), len(preds))
			}
			byPred := make(map[*Block]ValueID, len(preds))
			for i, p := range preds {
				byPred[p] = args[i]
			}
			next := make([]ValueID, 0, len(b.Preds))
			for _, p := range b.Preds {
				v, ok := byPred[p]
				if !ok {
					return bail("phi in block %d gained an unexpected predecessor", b.ID)
				}
				next = append(next, v)
			}
			in.Args = next
		}
	}
	return nil
}

// validate 校验 IR 的结构性不变式。任何一条不成立都返回错误，
// 调用方（optimizeFunction）会把错误当成"放弃优化"处理，
// 因此错误的字节码不会被交给 VM。
func (f *Function) validate() error {
	if f.Entry == nil {
		return bail("function %s has no entry block", f.Name)
	}
	blocks := make(map[*Block]bool, len(f.Blocks))
	for _, b := range f.Blocks {
		blocks[b] = true
	}
	for _, b := range f.Blocks {
		// 终止指令必须是最后一条，且只能出现在最后
		for i, in := range b.Instrs {
			if in.IsTerminator() && i != len(b.Instrs)-1 {
				return bail("block %d has instructions after a terminator", b.ID)
			}
		}
		if last := b.Last(); last == nil || !last.IsTerminator() {
			return bail("block %d does not end with a terminator", b.ID)
		}
		// 前驱/后继必须都指向存在的块
		for _, p := range b.Preds {
			if !blocks[p] {
				return bail("block %d lists a predecessor outside the function", b.ID)
			}
		}
		for _, s := range b.Succs {
			if !blocks[s] {
				return bail("block %d lists a successor outside the function", b.ID)
			}
		}
		// phi 必须在块首，且实参个数与前驱一致
		seenNonPhi := false
		for _, in := range b.Instrs {
			if in.Op == OpPhi {
				if seenNonPhi {
					return bail("block %d has a phi after a non-phi instruction", b.ID)
				}
				if len(in.Args) != len(b.Preds) {
					return bail("block %d: phi has %d args but block has %d preds",
						b.ID, len(in.Args), len(b.Preds))
				}
				continue
			}
			seenNonPhi = true
		}
	}

	// 每个被引用的值都必须有定义点
	for _, b := range f.Blocks {
		for _, in := range b.Instrs {
			for _, a := range in.Args {
				if a == NoValue {
					continue
				}
				if f.Def(a) == nil {
					return bail("value %d is used but never defined (block %d)", a, b.ID)
				}
			}
		}
	}
	_ = fmt.Sprint
	return nil
}
