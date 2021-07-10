package bytecode

import (
	"Interpreter/code"
	"Interpreter/object"
	"fmt"
	"strconv"
	"strings"
)

var RecursionLimit = 30

type Bytecode struct {
	Instruction code.Instructions
	Constants   []object.Object
	Functions   []object.Object
	Symbols     *SymbolTable
}

func (b *Bytecode) InsToString(ins code.Instructions, start, indent int) string {
	var sb strings.Builder
	for i := 0; i < len(ins); i++ {
		opcode := code.Opcode(ins[i])
		def, ok := code.Definitions[opcode]
		if ok {
			operand, offset := code.ReadOperand(def, ins[i+1:])
			format := strings.Repeat(" ", start) + " %-" + strconv.Itoa(indent) + "d %-18s %s"
			args := b.getArgs(def, operand)
			sb.WriteString(fmt.Sprintf(format, i, def.Name, args))
			i += offset
			if i != len(ins)-1 {
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

func (b *Bytecode) String() string {
	return b.InsToString(b.Instruction, 0, 4)
}

func (b *Bytecode) getArgs(def code.Definition, operand []int) string {
	var args string
	var idx int
	switch len(def.OperandWidth) {
	case 1:
		idx = operand[0]
		args = strconv.Itoa(operand[0])
	case 2:
		args = strconv.Itoa(operand[0]) + strconv.Itoa(operand[1])
	default:
		args = ""
	}
	switch def.Name {
	case "OpConstant":
		obj := b.Constants[idx]
		args = string(obj.Type()) + "(" + obj.Inspect() + ")"
	case "OpSetGlobal", "OpGetGlobal", "OpUpdateGlobal":
		if name, ok := b.Symbols.findByIndex(idx); ok {
			args = name
		}
	case "OpSetLocal", "OpGetLocal", "OpUpdateLocal":
		if name, ok := b.Symbols.Inner.findByIndex(idx); ok {
			args = name
		}
	case "OpClosure":
		fn := b.Functions[idx].(object.CompiledFunc)
		var argSb strings.Builder
		argSb.WriteString("Func {" + fn.FnName + "}")
		RecursionLimit--
		if RecursionLimit <= 0 {
			return args
		}
		argSb.WriteString("\n" + b.InsToString(fn.Instructions, (30-RecursionLimit)*2, 4))
		return argSb.String()
	case "OpGetBuiltin":
		builtIn := object.BuiltinFns[idx]
		args = builtIn.Name
	}
	return args
}
