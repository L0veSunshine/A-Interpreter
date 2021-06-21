package code

import (
	"Interpreter/object"
	"fmt"
	"strconv"
	"strings"
)

type Bytecode struct {
	Instruction Instructions
	Constants   []object.Object
}

func (b Bytecode) String() string {
	var sb strings.Builder
	for i := 0; i < len(b.Instruction); i++ {
		opcode := Opcode(b.Instruction[i])
		def, ok := definitions[opcode]
		if ok {
			operand, offset := ReadOperand(def, b.Instruction[i+1:])
			args := b.getArgs(def, operand)
			order := fmt.Sprintf(`%4d `, i)
			sb.WriteString(order + def.Name + " " + args)
			i += offset
			if i != len(b.Instruction)-1 {
				sb.WriteString("\n")
			}
		} else {
			continue
		}
	}
	return sb.String()
}

func (b *Bytecode) getArgs(def Definition, operand []int) string {
	var args string
	switch len(def.OperandWidth) {
	case 1:
		args = strconv.Itoa(operand[0])
	case 2:
		args = strconv.Itoa(operand[0]) + strconv.Itoa(operand[1])
	default:
		args = ""
	}
	switch def.Name {
	case "OpConstant":
		idx, e := strconv.Atoi(args)
		if e != nil {
			fmt.Println(e)
		}
		obj := b.Constants[idx]
		args += " ==> " + string(obj.Type()) + " " + obj.Inspect()
	case "OpUpdate":
		args = " => var " + args
	}
	return args
}
