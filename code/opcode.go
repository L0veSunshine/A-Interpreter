package code

type Opcode byte

const (
	_ Opcode = iota
	OpConstant
	OpPop

	OpTrue
	OpFalse

	OpAdd
	OpSub
	OpMul
	OpDiv
	OpFloor
	OpPow
	OpGT
	OpGTEq
	OpEqual
	OpNotEQ

	OpAnd
	OpOr
	OpNot

	OpMinus
	OpPlus

	OpJump
	OpJumpNotTrue

	OpNull

	OpGetGlobal
	OpSetGlobal
)
