package code

type Opcode byte

const (
	_ Opcode = iota
	OpConstant
	OpPop
	OpPrintTop

	OpTrue
	OpFalse

	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	OpFloorDiv
	OpPow
	OpGT
	OpGTEq
	OpLT
	OpLTEq
	OpEqual
	OpNotEQ

	OpNot

	OpMinus
	OpPlus

	OpJump
	OpJumpNotTrue

	OpNull

	OpReturnVal

	OpGetGlobal
	OpSetGlobal
	OpGetLocal
	OpSetLocal
	OpUpdateGlobal
	OpUpdateLocal

	OpGetBuiltin

	OpBuildArray
	OpMakeSlice
	OpMakeMap
	OpUpdate

	OpIndex

	OpCallFunc
	// OpCallMethod: [methodIdx u16][argc u8]
	// 一条指令完成"取方法实现 + 调用"：接收者在 sp-argc-1，
	// 方法实现按 (运行时类型, methodIdx) 在一次构建好的表里取。
	// 早期版本是 OpLoadMethod + OpCallMethod 两条，多一次派发、
	// 一次字符串 map 查找和一次方法对象的接口装箱。
	OpCallMethod
	OpClosure

	// 以下是融合指令（superinstruction）：由 code.Fuse 把常见的相邻指令
	// 合并成一条，减少解释器的派发次数。语义与原序列完全等价。
	//
	// OpBinFused:      [kindAB u8][binOp u8][idxA u16][idxB u16]
	//                  等价于「压入 A、压入 B、执行 binOp」，A/B 可以是
	//                  局部变量 / 常量 / 全局变量（kindA 在低 4 位）。
	OpBinFused
	// OpReturnOperand: [kind u8][idx u16]，等价于「压入操作数、返回」。
	OpReturnOperand
	// OpBinReturn:     [binOp u8]，等价于「执行 binOp、返回」。
	OpBinReturn
	// OpCmpBranch:     [kindAB u8][cmpOp u8][idxA u16][idxB u16][falseTarget u16]
	//                  等价于「比较 A、B，结果为假时跳到 falseTarget」。
	//                  这样就完全不需要把布尔结果物化到操作数栈上。
	OpCmpBranch
	// OpCallFuncConst: [fnIdx u16][nargs u8]
	//                  等价于「压入常量池第 fnIdx 项（函数对象）、压入 nargs 个
	//                  实参、调用」，但 callee 不进操作数栈——省掉每条 OpClosure。
	OpCallFuncConst
	// OpIndexBinFused: [kindAB u8][binOp u8][idxA u16][idxB u16]
	//                  等价于「计算 A binOp B（算术）作为下标，读取栈顶容器」，
	//                  容器留在栈上，下标不进栈，整数下标不做装箱。
	OpIndexBinFused
	// OpUpdateBinFused: 操作数布局同 OpIndexBinFused，
	//                  等价于「计算 A binOp B 作为下标，写入 容器[下标] = 值」。
	OpUpdateBinFused

	// OpCaptureClosure constructs a closure using the function's capture descriptors.
	OpCaptureClosure
	OpGetFree
	OpSetFree
)

// 融合指令里"操作数来源"的编号。
//
// 不要依赖这些值的大小顺序：它们只是编码。
const (
	OperandLocal  byte = 0
	OperandConst  byte = 1
	OperandGlobal byte = 2
)

// IsArithOp 判断是否是算术运算（走"算出一个新值"的语义）。
func IsArithOp(op Opcode) bool {
	switch op {
	case OpAdd, OpSub, OpMul, OpDiv, OpMod, OpFloorDiv, OpPow:
		return true
	}
	return false
}

// IsCompareOp 判断是否是比较运算（走"算出一个布尔值"的语义）。
func IsCompareOp(op Opcode) bool {
	switch op {
	case OpGT, OpGTEq, OpLT, OpLTEq, OpEqual, OpNotEQ:
		return true
	}
	return false
}
