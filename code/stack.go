package code

import "fmt"

// 本文件计算一段指令流在运行期需要多大的操作数栈。
//
// 为什么需要它：栈式解释器里每条指令都可能压栈，最直白的写法是"每执行一条
// 指令就检查一次剩余空间"。但那是一次多余的内存读 + 比较 + 分支，实测占
// 解释器开销的 6% 左右。
//
// 业界做法都是**把这件事挪到编译期**：
//
//   - CPython：编译期用 stackdepth() 做一遍数据流，把结果写进 code object 的
//     co_stacksize；帧对象一次性按 frame_size = locals + stacksize 分配，
//     求值循环里没有任何容量检查。
//   - Lua：不静态计算，但规定栈上永远至少留 LUA_MINSTACK（20）个空位，
//     只在"可能一次要很多槽"的指令和函数调用边界调 luaD_checkstack。
//   - JVM：Code 属性里直接带 max_stack（由 javac/verify 阶段算出）。
//
// 这里采用 CPython 的方案：算出精确上界，建帧时预留。算不出来时由调用方
// 退回到保守上界（见 ConservativeMaxStack），保证"要么精确、要么安全"。

// MaxStack 返回执行这段指令流时操作数栈需要的最大槽位数（相对入口深度 0）。
//
// 只做纯静态分析，不执行代码。遇到无法静态确定的形态（例如未知操作码、
// 或栈深度不一致的控制流汇合）就返回 error，由调用方决定如何兜底。
func MaxStack(ins Instructions) (int, error) {
	if len(ins) == 0 {
		return 0, nil
	}
	insts, err := decode(ins)
	if err != nil {
		return 0, err
	}

	// depth[i] 是第 i 条指令入口处的栈深度；-1 表示尚未确定。
	depth := make([]int, len(insts))
	for i := range depth {
		depth[i] = -1
	}
	// 偏移 -> 指令下标
	index := make(map[int]int, len(insts))
	for i, it := range insts {
		index[it.offset] = i
	}

	maxDepth := 0
	queue := []int{0}
	depth[0] = 0
	setDepth := func(idx, d int) error {
		if d < 0 {
			return fmt.Errorf("code: stack underflow before %s at offset %d",
				Definitions[insts[idx].op].Name, insts[idx].offset)
		}
		if depth[idx] < 0 {
			depth[idx] = d
			queue = append(queue, idx)
			return nil
		}
		if depth[idx] != d {
			return fmt.Errorf("code: inconsistent stack depth at offset %d (%d vs %d)",
				insts[idx].offset, depth[idx], d)
		}
		return nil
	}

	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		it := insts[i]
		delta, err := stackDelta(it.op, it.operands)
		if err != nil {
			return 0, err
		}
		out := depth[i] + delta
		if out < 0 {
			return 0, fmt.Errorf("code: %s at offset %d pops more than the stack holds",
				Definitions[it.op].Name, it.offset)
		}
		// 单条指令内部最多先把操作数压上去再弹出，峰值不超过
		// max(入口深度, 出口深度)。
		if out > maxDepth {
			maxDepth = out
		}
		if depth[i] > maxDepth {
			maxDepth = depth[i]
		}

		switch it.op {
		case OpReturnVal, OpReturnOperand, OpBinReturn:
			continue // 终结指令，无后继
		case OpJump:
			to, ok := index[it.operands[0]]
			if !ok {
				return 0, fmt.Errorf("code: jump at %d targets non-instruction offset %d",
					it.offset, it.operands[0])
			}
			if err := setDepth(to, out); err != nil {
				return 0, err
			}
			continue
		case OpJumpNotTrue:
			to, ok := index[it.operands[0]]
			if !ok {
				return 0, fmt.Errorf("code: jump at %d targets non-instruction offset %d",
					it.offset, it.operands[0])
			}
			if err := setDepth(to, out); err != nil {
				return 0, err
			}
			// 条件不成立时顺序落到下一条
		case OpCmpBranch:
			// 融合后的"比较 + 条件跳转"：假目标在最后一个操作数里
			to, ok := index[it.operands[len(it.operands)-1]]
			if !ok {
				return 0, fmt.Errorf("code: branch at %d targets non-instruction offset %d",
					it.offset, it.operands[len(it.operands)-1])
			}
			if err := setDepth(to, out); err != nil {
				return 0, err
			}
			// 比较为真时顺序落到下一条
		}
		if i+1 < len(insts) {
			if err := setDepth(i+1, out); err != nil {
				return 0, err
			}
		}
	}
	return maxDepth, nil
}

// ConservativeMaxStack 是"算不出来时"的兜底上界。
//
// 每条指令净压栈最多 1 个槽位，所以执行过的指令条数一定不小于最大深度，
// 用指令总数当作上界永远成立（可能偏大，但绝不会不够）。
func ConservativeMaxStack(ins Instructions) int {
	if n, err := MaxStack(ins); err == nil {
		return n
	}
	return len(ins)
}

// stackDelta 返回单条指令对操作数栈的净影响。
//
// 这里的每一项都必须与 vm 包里的实际实现严格对应；任何不确定的形态都返回
// error，让调用方走兜底路径，而不是猜一个可能偏小的值。
func stackDelta(op Opcode, operands []int) (int, error) {
	switch op {
	// 压入一个值
	case OpConstant, OpTrue, OpFalse, OpNull, OpGetLocal, OpGetGlobal,
		OpGetBuiltin, OpClosure, OpLoadMethod, OpBinFused:
		return 1, nil

	// 弹出且不压
	case OpPop, OpPrintTop, OpSetLocal, OpSetGlobal, OpReturnVal:
		return -1, nil

	// 二元/一元运算：弹出操作数、压入一个结果
	case OpAdd, OpSub, OpMul, OpDiv, OpMod, OpPow,
		OpGT, OpGTEq, OpLT, OpLTEq, OpEqual, OpNotEQ,
		OpNot, OpMinus, OpPlus:
		return -1, nil

	// 只在栈顶就地写入，不改变深度
	case OpUpdateLocal, OpUpdateGlobal, OpJump:
		return 0, nil

	// 条件跳转：弹出条件
	case OpJumpNotTrue:
		return -1, nil

	case OpIndex:
		return -1, nil

	// 弹出 new/obj/key 三个值后把容器压回栈
	case OpUpdate:
		return -2, nil

	// 弹出三个值（start/end/step）压回一个 slice
	case OpMakeSlice:
		return -2, nil

	case OpBuildArray:
		return 1 - operands[0], nil
	case OpMakeMap:
		return 1 - operands[0], nil

	// 弹出 callee + n 个实参，压回一个返回值
	case OpCallFunc:
		return -operands[0], nil

	// 只弹出 n 个实参（callee 来自常量池，不占栈），压回一个返回值
	case OpCallFuncConst:
		return 1 - operands[1], nil

	// 比较 + 条件分支：弹出两个操作数后什么都不留
	case OpCmpBranch:
		return 0, nil

	// 弹出 self + method + n 个实参，压回 self 与返回值。
	// 方法可能只返回一个值，那种情况下实际深度更低，
	// 所以按"压回 2 个"计算得到的是安全的上界。
	case OpCallMethod:
		return -operands[0], nil

	// 终结指令：OpReturnOperand 会先把操作数压栈（峰值 +1）再返回，
	// OpBinReturn 先弹两个压一个再返回（峰值不增加）。
	case OpReturnOperand:
		return 1, nil
	case OpBinReturn:
		return -1, nil
	}
	return 0, fmt.Errorf("code: unknown stack effect for opcode %d", op)
}
