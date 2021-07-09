package code

import (
	"encoding/binary"
)

type Instructions []byte

type Definition struct {
	Name         string
	OperandWidth []int
}

var Definitions = map[Opcode]Definition{
	OpConstant:     {"OpConstant", []int{2}},
	OpPop:          {"OpPop", []int{}},
	OpTop:          {"OpTop", []int{}},
	OpAdd:          {"OpAdd", []int{}},
	OpSub:          {"OpSub", []int{}},
	OpMul:          {"OpMul", []int{}},
	OpDiv:          {"OpDiv", []int{}},
	OpMinus:        {"OpMinus", []int{}},
	OpPlus:         {"OpPlus", []int{}},
	OpMod:          {"OpMod", []int{}},
	OpPow:          {"OpPow", []int{}},
	OpEqual:        {"OpEqual", []int{}},
	OpNotEQ:        {"OpNotEQ", []int{}},
	OpGT:           {"OpGT", []int{}},
	OpGTEq:         {"OpGTEq", []int{}},
	OpAnd:          {"OpAnd", []int{}},
	OpOr:           {"OpOr", []int{}},
	OpNot:          {"OpNot", []int{}},
	OpTrue:         {"OpTrue", []int{}},
	OpFalse:        {"OpFalse", []int{}},
	OpJump:         {"OpJump", []int{2}},
	OpJumpNotTrue:  {"OpJumpNotTrue", []int{2}},
	OpNull:         {"OpNull", []int{}},
	OpGetGlobal:    {"OpGetGlobal", []int{2}},
	OpSetGlobal:    {"OpSetGlobal", []int{2}},
	OpReturn:       {"OpReturn", []int{}},
	OpReturnVal:    {"OpReturnVal", []int{}},
	OpUpdateGlobal: {"OpUpdateGlobal", []int{2}},
	OpUpdateLocal:  {"OpUpdateLocal", []int{2}},
	OpGetLocal:     {"OpGetLocal", []int{2}},
	OpSetLocal:     {"OpSetLocal", []int{2}},
	OpGetBuiltin:   {"OpGetBuiltin", []int{1}},
	OpCallFunc:     {"OpCallFunc", []int{1}},
	OpClosure:      {"OpClosure", []int{1}},
}

func Make(op Opcode, operand ...int) []byte {
	def, ok := Definitions[op]
	if !ok {
		return []byte{}
	}
	var insLen = 1
	for _, w := range def.OperandWidth {
		insLen += w
	}
	instruction := make([]byte, insLen)
	instruction[0] = byte(op)
	offset := 1
	for i, o := range operand {
		width := def.OperandWidth[i]
		switch width {
		case 2:
			binary.BigEndian.PutUint16(instruction[offset:], uint16(o))
		case 1:
			instruction[offset] = byte(o)
		}
		offset += width
	}
	return instruction
}

func ReadOperand(def Definition, ins Instructions) ([]int, int) {
	operands := make([]int, len(def.OperandWidth))
	offset := 0
	for i, width := range def.OperandWidth {
		switch width {
		case 2:
			operands[i] = int(ReadUint16(ins[offset:]))
		case 1:
			operands[i] = int(ReadUint8(ins[offset:]))
		}
		offset += width
	}
	return operands, offset
}

func ReadUint8(ins Instructions) uint8 {
	return ins[0]
}

func ReadUint16(ins Instructions) uint16 {
	return binary.BigEndian.Uint16(ins)
}
