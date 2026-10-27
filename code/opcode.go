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
)
