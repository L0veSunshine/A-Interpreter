package ir

import (
	"math"

	"Interpreter/code"
	"Interpreter/object"
)

// 本文件是"基础 SSA 优化"：常量折叠、分支化简、块内 CSE、死代码删除。
//
// 所有折叠语义必须与 VM 完全一致，否则优化会改变程序结果。
// 因此 foldBinary 逐个对照 vm.executeBinOp / compareBinOp 的分派规则：
//
//	两边都是 Int          → 走整数路径（除法/幂返回 Float）
//	只要有一边是 String   → 走字符串路径（+ 是拼接，其余报错）
//	只要有一边是 Float    → 走浮点路径
//	其余（None/Bool 等）  → 只有 == / != 有意义

// optimize 跑一轮基础优化；会重复到不动点（折叠可能连锁产生新的常量）。
func (f *Function) optimize() error {
	for pass := 0; pass < 8; pass++ {
		changed := false
		if f.foldConstants() {
			changed = true
		}
		if f.simplifyBranches() {
			changed = true
		}
		if f.cse() {
			changed = true
		}
		if f.removeDeadCode() {
			changed = true
		}
		if f.removeUnreachable() {
			changed = true
		}
		if !changed {
			break
		}
	}
	return nil
}

// foldConstants 把操作数全为常量的指令直接算出结果。
func (f *Function) foldConstants() bool {
	changed := false
	for _, b := range f.Blocks {
		for _, in := range b.Instrs {
			switch in.Op {
			case OpBin:
				if len(in.Args) != 2 {
					continue
				}
				x, ok1 := f.constOf(in.Args[0])
				y, ok2 := f.constOf(in.Args[1])
				if !ok1 || !ok2 {
					continue
				}
				res, ok := foldBinary(code.Opcode(in.Imm), x, y)
				if !ok {
					continue
				}
				f.rewriteConst(in, res)
				changed = true
			case OpUn:
				if len(in.Args) != 1 {
					continue
				}
				x, ok := f.constOf(in.Args[0])
				if !ok {
					continue
				}
				res, ok := foldUnary(code.Opcode(in.Imm), x)
				if !ok {
					continue
				}
				f.rewriteConst(in, res)
				changed = true
			}
		}
	}
	return changed
}

// rewriteConst 把一条指令改写成"产生常量 res"。
func (f *Function) rewriteConst(in *Instr, res object.Object) {
	idx := f.pool.intern(res)
	in.Op = OpConst
	in.Imm = idx
	in.Args = nil
	f.types[in.Dst] = typeOfObject(res)
}

// constOf 返回该值是否为常量，以及常量本身。
func (f *Function) constOf(v ValueID) (object.Object, bool) {
	d := f.Def(v)
	if d == nil || d.Op != OpConst {
		return nil, false
	}
	return f.pool.objs[d.Imm], true
}

// simplifyBranches 把"条件恒为常量"的分支变成无条件跳转。
func (f *Function) simplifyBranches() bool {
	changed := false
	for _, b := range f.Blocks {
		last := b.Last()
		if last == nil || last.Op != OpBranch || len(last.Args) != 1 {
			continue
		}
		c, ok := f.constOf(last.Args[0])
		if !ok {
			continue
		}
		truthy, ok := constTruthy(c)
		if !ok {
			continue
		}
		if truthy {
			last.Op = OpJump
			last.Imm = last.Imm2
		} else {
			last.Op = OpJump
		}
		last.Args = nil
		changed = true
	}
	return changed
}

// constTruthy 复刻 vm.objToNativeBool 的真值语义。
func constTruthy(o object.Object) (bool, bool) {
	switch v := o.(type) {
	case object.Boolean:
		return v.Value, true
	case object.Int:
		return v.Value != 0, true
	case object.Float:
		return v.Value != 0, true
	case object.Null:
		return false, true
	case object.String:
		return len(v.Value) > 0, true
	}
	return false, false
}

// cse 做块内公共子表达式消除。
//
// 只处理"纯计算 + 读堆"两类：OpBin / OpUn / OpIndex。
// OpCall、OpUpdate、OpStoreGlobal 是屏障：它们可能改写堆或全局，
// 跨过它们复用 OpIndex 会读到旧值。
// 数组/map 字面量不参与 CSE——复用会让两个元素变成同一个对象，
// 之后对其中一个做原地修改就会串味。
func (f *Function) cse() bool {
	changed := false
	replace := map[ValueID]ValueID{}

	for _, b := range f.Blocks {
		available := map[string]ValueID{}
		for _, in := range b.Instrs {
			// 先替换实参
			for i, a := range in.Args {
				in.Args[i] = resolveValue(a, replace)
			}

			switch in.Op {
			case OpCall, OpUpdate, OpStoreGlobal:
				available = map[string]ValueID{}
				continue
			}
			if !cseEligible(in) {
				continue
			}
			key := cseKey(in)
			if prev, ok := available[key]; ok {
				// 复用已有值，本条指令变成空操作
				replace[in.Dst] = prev
				in.Op = OpNop
				in.Args = nil
				changed = true
				continue
			}
			available[key] = in.Dst
		}
	}

	if len(replace) == 0 {
		return changed
	}
	for _, b := range f.Blocks {
		for _, in := range b.Instrs {
			for i, a := range in.Args {
				in.Args[i] = resolveValue(a, replace)
			}
		}
	}
	f.dropNops()
	return true
}

func cseEligible(in *Instr) bool {
	switch in.Op {
	case OpBin, OpUn, OpIndex:
		return in.Dst != NoValue
	}
	return false
}

func cseKey(in *Instr) string {
	key := in.Op.String()
	if in.Op == OpBin || in.Op == OpUn {
		key += ":" + code.Definitions[code.Opcode(in.Imm)].Name
	}
	for _, a := range in.Args {
		key += ":"
		key += itoa(int(a))
	}
	return key
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [12]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// removeDeadCode 反复删除"结果无人使用"的纯指令。
func (f *Function) removeDeadCode() bool {
	removedAny := false
	for {
		uses := make([]int, int(f.numValue)+1)
		for _, b := range f.Blocks {
			for _, in := range b.Instrs {
				for _, a := range in.Args {
					if a != NoValue && int(a) < len(uses) {
						uses[a]++
					}
				}
			}
		}

		removed := false
		for _, b := range f.Blocks {
			out := b.Instrs[:0]
			for _, in := range b.Instrs {
				if in.Op == OpNop {
					removed = true
					continue
				}
				if in.CannotTrap() && in.Dst != NoValue && in.Op != OpInit &&
					int(in.Dst) < len(uses) && uses[in.Dst] == 0 {
					removed = true
					continue
				}
				out = append(out, in)
			}
			b.Instrs = out
		}
		if !removed {
			return removedAny
		}
		removedAny = true
	}
}

// removeUnreachable 删掉从入口不可达的块，并把跳转目标重新编号。
func (f *Function) removeUnreachable() bool {
	if f.Entry == nil {
		return false
	}
	reachable := map[*Block]bool{}
	var walk func(b *Block)
	walk = func(b *Block) {
		if reachable[b] {
			return
		}
		reachable[b] = true
		for _, s := range b.Succs {
			walk(s)
		}
	}
	walk(f.Entry)
	if len(reachable) == len(f.Blocks) {
		return false
	}

	// 先记录"旧 ID -> 新 ID"，再重新编号
	newID := make(map[int]int, len(reachable))
	kept := make([]*Block, 0, len(reachable))
	for _, b := range f.Blocks {
		if !reachable[b] {
			continue
		}
		newID[b.ID] = len(kept)
		kept = append(kept, b)
	}
	f.Blocks = kept
	for _, b := range f.Blocks {
		b.ID = newID[b.ID]
	}
	for _, b := range f.Blocks {
		last := b.Last()
		if last == nil {
			continue
		}
		switch last.Op {
		case OpJump:
			last.Imm = newID[last.Imm]
		case OpBranch:
			last.Imm = newID[last.Imm]
			last.Imm2 = newID[last.Imm2]
		}
	}
	f.linkBlocks()
	return true
}

// dropNops 删除被 CSE 标记为空的指令。
func (f *Function) dropNops() {
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

// ---------------------------------------------------------------------------
// 常量折叠的求值规则（必须与 VM 完全一致）
// ---------------------------------------------------------------------------

func foldBinary(op code.Opcode, x, y object.Object) (object.Object, bool) {
	// 1) 两边都是 Int：走整数路径
	if isInt(x) && isInt(y) {
		a := int(x.(object.Int).Value)
		b := int(y.(object.Int).Value)
		switch op {
		case code.OpAdd:
			return object.Int{Value: a + b}, true
		case code.OpSub:
			return object.Int{Value: a - b}, true
		case code.OpMul:
			return object.Int{Value: a * b}, true
		case code.OpDiv:
			if b == 0 {
				return nil, false // 让运行时去报除零
			}
			return object.Float{Value: float64(a) / float64(b)}, true
		case code.OpMod:
			if b == 0 {
				return nil, false
			}
			return object.Int{Value: a % b}, true
		case code.OpPow:
			return object.Float{Value: math.Pow(float64(a), float64(b))}, true
		}
		// 比较
		if res, ok := foldNumCompare(op, float64(a), float64(b)); ok {
			return res, true
		}
		return nil, false
	}

	// 2) 任一边是 String：走字符串路径
	if isString(x) || isString(y) {
		if op == code.OpAdd {
			return object.String{Value: []rune(x.Inspect() + y.Inspect())}, true
		}
		if isString(x) && isString(y) {
			a := string(x.(object.String).Value)
			b := string(y.(object.String).Value)
			switch op {
			case code.OpEqual:
				return object.Boolean{Value: a == b}, true
			case code.OpNotEQ:
				return object.Boolean{Value: a != b}, true
			case code.OpGT:
				return object.Boolean{Value: a > b}, true
			case code.OpGTEq:
				return object.Boolean{Value: a >= b}, true
			case code.OpLT:
				return object.Boolean{Value: a < b}, true
			case code.OpLTEq:
				return object.Boolean{Value: a <= b}, true
			}
		}
		return nil, false
	}

	// 3) 任一边是 Float：走浮点路径
	if isNumericObj(x) && isNumericObj(y) {
		a, _ := numericValue(x)
		b, _ := numericValue(y)
		switch op {
		case code.OpAdd:
			return object.Float{Value: a + b}, true
		case code.OpSub:
			return object.Float{Value: a - b}, true
		case code.OpMul:
			return object.Float{Value: a * b}, true
		case code.OpDiv:
			if b == 0 {
				return nil, false
			}
			return object.Float{Value: a / b}, true
		case code.OpMod:
			if b == 0 {
				return nil, false
			}
			return object.Float{Value: math.Mod(a, b)}, true
		case code.OpPow:
			return object.Float{Value: math.Pow(a, b)}, true
		}
		if res, ok := foldNumCompare(op, a, b); ok {
			return res, true
		}
		return nil, false
	}

	// 4) 其它类型只处理 == / !=
	switch op {
	case code.OpEqual:
		return object.Boolean{Value: object.RichCompare(x, y)}, true
	case code.OpNotEQ:
		return object.Boolean{Value: !object.RichCompare(x, y)}, true
	}
	return nil, false
}

func foldNumCompare(op code.Opcode, a, b float64) (object.Object, bool) {
	switch op {
	case code.OpEqual:
		return object.Boolean{Value: a == b}, true
	case code.OpNotEQ:
		return object.Boolean{Value: a != b}, true
	case code.OpGT:
		return object.Boolean{Value: a > b}, true
	case code.OpGTEq:
		return object.Boolean{Value: a >= b}, true
	case code.OpLT:
		return object.Boolean{Value: a < b}, true
	case code.OpLTEq:
		return object.Boolean{Value: a <= b}, true
	}
	return nil, false
}

func foldUnary(op code.Opcode, x object.Object) (object.Object, bool) {
	switch v := x.(type) {
	case object.Int:
		switch op {
		case code.OpMinus:
			return object.Int{Value: -v.Value}, true
		case code.OpPlus:
			return v, true
		}
	case object.Float:
		switch op {
		case code.OpMinus:
			return object.Float{Value: -v.Value}, true
		case code.OpPlus:
			return v, true
		}
	case object.Boolean:
		if op == code.OpNot {
			return object.Boolean{Value: !v.Value}, true
		}
	case object.Null:
		if op == code.OpNot {
			return object.Boolean{Value: true}, true
		}
	}
	return nil, false
}

func isInt(o object.Object) bool {
	_, ok := o.(object.Int)
	return ok
}

func isString(o object.Object) bool {
	_, ok := o.(object.String)
	return ok
}

func isNumericObj(o object.Object) bool {
	switch o.(type) {
	case object.Int, object.Float:
		return true
	}
	return false
}

func numericValue(o object.Object) (float64, bool) {
	switch v := o.(type) {
	case object.Int:
		return float64(v.Value), true
	case object.Float:
		return v.Value, false
	}
	return 0, false
}
