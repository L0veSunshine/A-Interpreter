package bytecode

import (
	"Interpreter/code"
	"Interpreter/object"
	"fmt"
	"strconv"
	"strings"
)

type Bytecode struct {
	Instruction code.Instructions
	Constants   []object.Object
}

func (b *Bytecode) InsToString(ins code.Instructions, indent int) string {
	var sb strings.Builder
	for i := 0; i < len(ins); i++ {
		opcode := code.Opcode(ins[i])
		def, ok := code.Definitions[opcode]
		if ok {
			operand, offset := code.ReadOperand(def, ins[i+1:])
			format := "%" + strconv.Itoa(indent) + "d "
			args := b.getArgs(def, operand)
			order := fmt.Sprintf(format, i)
			sb.WriteString(order + def.Name + " " + args)
			i += offset
			if i != len(ins)-1 {
				sb.WriteString("\n")
			}
		} else {
			continue
		}
	}
	return sb.String()
}

func (b *Bytecode) String() string {
	return b.InsToString(b.Instruction, 4)
}

func (b *Bytecode) getArgs(def code.Definition, operand []int) string {
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
		if obj.Type() == object.CompiledFuncObj {
			var argSb strings.Builder
			cf := obj.(object.CompiledFunc)
			argSb.WriteString("=> Func Def\n")
			argSb.WriteString(b.InsToString(cf.Instructions, 8))
			return argSb.String()
		} else {
			args += " ==> " + string(obj.Type()) + " " + obj.Inspect()
		}
	case "OpUpdate":
		args = " => var " + args
	}
	return args
}
