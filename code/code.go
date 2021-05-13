package code

import "encoding/binary"

type Instructions []byte

type Definition struct {
	Name         string
	OperandWidth []int
}

var definitions = map[Opcode]Definition{
	OpConstant: {"OpConstant", []int{2}},
	OpPop:      {"OpPop", []int{}},
	OpAdd:      {"OpAdd", []int{}},
	OpSub:      {"OpSub", []int{}},
	OpMul:      {"OpMul", []int{}},
	OpDiv:      {"OpDiv", []int{}},
	OpMinus:    {"OpMinus", []int{}},
	OpPlus:     {"OpPlus", []int{}},
}

func Make(op Opcode, operand ...int) []byte {
	def, ok := definitions[op]
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