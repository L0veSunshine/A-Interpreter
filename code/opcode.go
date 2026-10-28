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
	OpLoadMethod
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
	case OpAdd, OpSub, OpMul, OpDiv, OpMod, OpPow:
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
