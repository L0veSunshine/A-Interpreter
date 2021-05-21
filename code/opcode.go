package code

type Opcode byte

const (
	_ Opcode = iota
	OpConstant
	OpPop

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
)
