package ir

import (
	"Interpreter/cfg"
	"Interpreter/code"
	"Interpreter/object"
)

// buildFunction 把一段栈字节码提升成 IR。
//
// 做法（stack-to-register promotion）：按 CFG 逐块模拟操作数栈，
// 每次 push 都分配一个新的虚拟值，于是栈上每个位置都有名字，
// `a+b` 就从"压 a、压 b、Add"变成 `t = add a, b` 这样的三地址码。
//
// 只接受"块边界上操作数栈为空"的函数。当前编译器的语句级代码生成
// 满足这个条件（语句净栈高度为 0），因此绝大多数函数都能优化；
// 万一遇到不满足的形态，返回 unsupported 让上层回退到未优化字节码。
func buildFunction(name string, params int, ins code.Instructions,
	lines []code.LineEntry, pool *constPool, isMain bool) (*Function, error) {
	g, err := cfg.Build(ins)
	if err != nil {
		return nil, err
	}
	fn := &Function{Name: name, Params: params, pool: pool, isMain: isMain}
	if g.Entry == nil {
		return nil, bail("empty instruction stream")
	}

	byOffset := make(map[int]*Block, len(g.Blocks))
	for _, cb := range g.Blocks {
		if !cb.Reachable {
			continue
		}
		b := &Block{ID: len(fn.Blocks), Offset: cb.Start}
		fn.Blocks = append(fn.Blocks, b)
		byOffset[cb.Start] = b
	}
	fn.Entry = byOffset[g.Entry.Start]
	if fn.Entry == nil {
		return nil, bail("entry block missing")
	}

	// 形参在入口处已经在槽位里：为它们建 OpInit，作为 SSA 定义的起点。
	inits := make([]*Instr, 0, params)
	for i := 0; i < params; i++ {
		v := fn.NewValue()
		in := &Instr{Op: OpInit, Dst: v, Imm: i}
		fn.defs[v] = in
		inits = append(inits, in)
	}

	for _, cb := range g.Blocks {
		if !cb.Reachable {
			continue
		}
		b := byOffset[cb.Start]
		bld := &builder{fn: fn, block: b, byOffset: byOffset, lines: lines}
		if err := bld.run(cb); err != nil {
			return nil, err
		}
		fn.hasCall = fn.hasCall || bld.sawCall
	}
	if len(inits) > 0 {
		fn.Entry.Instrs = append(inits, fn.Entry.Instrs...)
	}

	fn.linkBlocks()
	return fn, nil
}

type builder struct {
	fn       *Function
	block    *Block
	byOffset map[int]*Block
	stack    []ValueID
	sawCall  bool
	// lines 是原始字节码的行号表，用于给每条 IR 指令精确打上行号
	lines   []code.LineEntry
	curLine int
}

func (b *builder) push(v ValueID) { b.stack = append(b.stack, v) }

func (b *builder) pop() (ValueID, error) {
	if len(b.stack) == 0 {
		return NoValue, bail("operand stack underflow at block %d", b.block.ID)
	}
	v := b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]
	return v, nil
}

func (b *builder) top() (ValueID, error) {
	if len(b.stack) == 0 {
		return NoValue, bail("operand stack underflow at block %d", b.block.ID)
	}
	return b.stack[len(b.stack)-1], nil
}

// popN 按原顺序返回栈顶 n 个值（最左边的先被压入）。
func (b *builder) popN(n int) ([]ValueID, error) {
	if len(b.stack) < n {
		return nil, bail("operand stack underflow at block %d", b.block.ID)
	}
	out := make([]ValueID, n)
	copy(out, b.stack[len(b.stack)-n:])
	b.stack = b.stack[:len(b.stack)-n]
	return out, nil
}

func (b *builder) emit(in *Instr) *Instr {
	if in.Line == 0 {
		in.Line = b.curLine
	}
	return b.fn.Emit(b.block, in)
}

// blockRef 把字节码偏移解析成 IR 块 ID。
func (b *builder) blockRef(offset int) (int, bool) {
	target, ok := b.byOffset[offset]
	if !ok {
		return 0, false
	}
	return target.ID, true
}

func (b *builder) run(cb *cfg.Block) error {
	terminated := false
	for _, op := range cb.Ops {
		if terminated {
			return bail("instructions after terminator at offset %d", op.Offset)
		}
		b.curLine = code.LineAt(b.lines, op.Offset)
		done, err := b.step(op)
		if err != nil {
			return err
		}
		terminated = done
	}

	if !terminated {
		// 块自然结束：顺序落到下一个块；没有下一个块说明函数到此结束
		b.curLine = code.LineAt(b.lines, cb.End)
		if target, ok := b.blockRef(cb.End); ok {
			b.emit(&Instr{Op: OpJump, Imm: target})
		} else {
			b.emit(&Instr{Op: OpExit})
		}
	}
	if len(b.stack) != 0 {
		// 有值跨块存活：需要"栈上的 phi"，收益低且容易出错，直接放弃优化
		return bail("value flows across block boundary in block %d", b.block.ID)
	}
	return nil
}

func (b *builder) binOp(bytecodeOp code.Opcode) error {
	right, err := b.pop()
	if err != nil {
		return err
	}
	left, err := b.pop()
	if err != nil {
		return err
	}
	v := b.fn.NewValue()
	b.emit(&Instr{Op: OpBin, Dst: v, Args: []ValueID{left, right}, Imm: int(bytecodeOp)})
	b.push(v)
	return nil
}

func (b *builder) unOp(bytecodeOp code.Opcode) error {
	operand, err := b.pop()
	if err != nil {
		return err
	}
	v := b.fn.NewValue()
	b.emit(&Instr{Op: OpUn, Dst: v, Args: []ValueID{operand}, Imm: int(bytecodeOp)})
	b.push(v)
	return nil
}

func (b *builder) constant(o object.Object) ValueID {
	idx := b.fn.pool.intern(o)
	v := b.fn.NewValue()
	b.emit(&Instr{Op: OpConst, Dst: v, Imm: idx})
	return v
}

func (b *builder) step(op cfg.Op) (bool, error) {
	switch op.Opcode {
	case code.OpConstant:
		v := b.fn.NewValue()
		b.emit(&Instr{Op: OpConst, Dst: v, Imm: op.Operands[0]})
		b.push(v)
		return false, nil

	case code.OpTrue:
		b.push(b.constant(object.Boolean{Value: true}))
		return false, nil
	case code.OpFalse:
		b.push(b.constant(object.Boolean{Value: false}))
		return false, nil
	case code.OpNull:
		b.push(b.constant(object.Null{}))
		return false, nil

	case code.OpPop:
		v, err := b.pop()
		if err != nil {
			return false, err
		}
		// OpPop 不只是"丢弃"：VM 会把它记成 lastPop（REPL/测试依赖这个值）。
		// 因此 IR 里保留一个显式的 sink，死代码删除不会把整棵表达式树删光。
		b.emit(&Instr{Op: OpSink, Args: []ValueID{v}})
		return false, nil

	case code.OpAdd, code.OpSub, code.OpMul, code.OpDiv, code.OpMod, code.OpPow,
		code.OpGT, code.OpGTEq, code.OpLT, code.OpLTEq, code.OpEqual, code.OpNotEQ:
		return false, b.binOp(op.Opcode)

	case code.OpMinus, code.OpPlus, code.OpNot:
		return false, b.unOp(op.Opcode)

	case code.OpJump:
		target, ok := b.blockRef(op.Operands[0])
		if !ok {
			// 跳到指令流末尾 = 函数结束
			b.emit(&Instr{Op: OpExit})
			return true, nil
		}
		b.emit(&Instr{Op: OpJump, Imm: target})
		return true, nil

	case code.OpJumpNotTrue:
		cond, err := b.pop()
		if err != nil {
			return false, err
		}
		falseTarget, ok := b.blockRef(op.Operands[0])
		if !ok {
			return false, bail("conditional jump targets offset %d outside the function", op.Operands[0])
		}
		trueTarget, ok := b.byOffset[op.Offset+op.Width]
		if !ok {
			return false, bail("conditional jump at %d falls through to a non-block", op.Offset)
		}
		b.emit(&Instr{Op: OpBranch, Args: []ValueID{cond}, Imm: falseTarget, Imm2: trueTarget.ID})
		return true, nil

	case code.OpReturnVal:
		v, err := b.pop()
		if err != nil {
			return false, err
		}
		b.emit(&Instr{Op: OpReturn, Args: []ValueID{v}})
		return true, nil

	case code.OpGetGlobal:
		v := b.fn.NewValue()
		b.emit(&Instr{Op: OpLoadGlobal, Dst: v, Imm: op.Operands[0]})
		b.push(v)
		return false, nil
	case code.OpSetGlobal:
		v, err := b.pop()
		if err != nil {
			return false, err
		}
		b.emit(&Instr{Op: OpStoreGlobal, Args: []ValueID{v}, Imm: op.Operands[0]})
		return false, nil
	case code.OpUpdateGlobal:
		// 写入但保留在栈上：建模成"存储 + 重新读回"。
		// 这样存储本身消耗掉原值，原值若只有这一次使用就能直接inline到栈上。
		v, err := b.pop()
		if err != nil {
			return false, err
		}
		b.emit(&Instr{Op: OpStoreGlobal, Args: []ValueID{v}, Imm: op.Operands[0]})
		nv := b.fn.NewValue()
		b.emit(&Instr{Op: OpLoadGlobal, Dst: nv, Imm: op.Operands[0]})
		b.push(nv)
		return false, nil

	case code.OpGetLocal:
		v := b.fn.NewValue()
		b.emit(&Instr{Op: OpLoadLocal, Dst: v, Imm: op.Operands[0]})
		b.push(v)
		return false, nil
	case code.OpSetLocal:
		v, err := b.pop()
		if err != nil {
			return false, err
		}
		b.emit(&Instr{Op: OpStoreLocal, Args: []ValueID{v}, Imm: op.Operands[0]})
		return false, nil
	case code.OpUpdateLocal:
		// 同 OpUpdateGlobal：存储 + 读回。
		v, err := b.pop()
		if err != nil {
			return false, err
		}
		b.emit(&Instr{Op: OpStoreLocal, Args: []ValueID{v}, Imm: op.Operands[0]})
		nv := b.fn.NewValue()
		b.emit(&Instr{Op: OpLoadLocal, Dst: nv, Imm: op.Operands[0]})
		b.push(nv)
		return false, nil

	case code.OpGetBuiltin:
		v := b.fn.NewValue()
		b.emit(&Instr{Op: OpBuiltin, Dst: v, Imm: op.Operands[0]})
		b.push(v)
		return false, nil

	case code.OpBuildArray:
		elems, err := b.popN(op.Operands[0])
		if err != nil {
			return false, err
		}
		v := b.fn.NewValue()
		b.emit(&Instr{Op: OpMakeArray, Dst: v, Args: elems})
		b.push(v)
		return false, nil

	case code.OpMakeMap:
		n := op.Operands[0]
		if n%2 != 0 {
			return false, bail("map literal with odd slot count %d", n)
		}
		pairs, err := b.popN(n)
		if err != nil {
			return false, err
		}
		v := b.fn.NewValue()
		b.emit(&Instr{Op: OpMakeMap, Dst: v, Args: pairs})
		b.push(v)
		return false, nil

	case code.OpIndex:
		key, err := b.pop()
		if err != nil {
			return false, err
		}
		obj, err := b.pop()
		if err != nil {
			return false, err
		}
		v := b.fn.NewValue()
		b.emit(&Instr{Op: OpIndex, Dst: v, Args: []ValueID{obj, key}})
		b.push(v)
		return false, nil

	case code.OpUpdate:
		key, err := b.pop()
		if err != nil {
			return false, err
		}
		obj, err := b.pop()
		if err != nil {
			return false, err
		}
		target, err := b.pop()
		if err != nil {
			return false, err
		}
		// VM 在 OpUpdate 之后会把容器重新压回栈；因为 Array/Map 是原地修改，
		// 压回的就是同一个对象，所以直接复用 obj 这个值，不引入新值。
		b.emit(&Instr{Op: OpUpdate, Args: []ValueID{target, obj, key}})
		b.push(obj)
		return false, nil

	case code.OpClosure:
		v := b.fn.NewValue()
		b.emit(&Instr{Op: OpMakeClosure, Dst: v, Imm: op.Operands[0]})
		b.push(v)
		return false, nil

	case code.OpCallFunc:
		n := op.Operands[0]
		args, err := b.popN(n)
		if err != nil {
			return false, err
		}
		callee, err := b.pop()
		if err != nil {
			return false, err
		}
		all := make([]ValueID, 0, n+1)
		all = append(all, callee)
		all = append(all, args...)
		v := b.fn.NewValue()
		b.emit(&Instr{Op: OpCall, Dst: v, Args: all})
		b.push(v)
		b.sawCall = true
		return false, nil

	case code.OpPrintTop, code.OpLoadMethod, code.OpCallMethod, code.OpMakeSlice:
		return false, bail("opcode %s is not supported by the optimizer", op.Name())
	}
	return false, bail("unknown opcode %d", op.Opcode)
}

// linkBlocks 根据终结指令填充前驱/后继。
func (f *Function) linkBlocks() {
	for _, b := range f.Blocks {
		b.Succs = nil
		b.Preds = nil
	}
	for _, b := range f.Blocks {
		last := b.Last()
		if last == nil {
			continue
		}
		switch last.Op {
		case OpJump:
			f.addEdge(b, last.Imm)
		case OpBranch:
			f.addEdge(b, last.Imm)
			f.addEdge(b, last.Imm2)
		}
	}
}

func (f *Function) addEdge(from *Block, targetID int) {
	if targetID < 0 || targetID >= len(f.Blocks) {
		return
	}
	to := f.Blocks[targetID]
	for _, s := range from.Succs {
		if s == to {
			return
		}
	}
	from.Succs = append(from.Succs, to)
	to.Preds = append(to.Preds, from)
}
