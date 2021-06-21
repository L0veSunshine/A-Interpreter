package code

type Opcode byte

const (
	_ Opcode = iota
	OpConstant
	OpPop
	OpTop

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
	OpUpdate
)
