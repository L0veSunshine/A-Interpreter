package bytecode

import (
	"Interpreter/code"
	"Interpreter/object"
	"Interpreter/parser"
	"fmt"
	"strconv"
	"strings"
)

type Bytecode struct {
	Instruction code.Instructions
	Constants   []object.Object
	Symbols     *parser.SymTable
}

func (b *Bytecode) Ins() string {
	ls := strings.Repeat("=", 50) + "\n"
	var sb strings.Builder
	sb.WriteString(ls)
	sb.WriteString(b.decodeFunc())

	sb.WriteString(fmt.Sprintf("%30s\n", "<Main Function>"))
	//sb.WriteString(ls)
	sb.WriteString(b.String() + "\n")
	sb.WriteString(ls)
	return sb.String()
}

func (b *Bytecode) InsToString(ins code.Instructions, start, indent int, scope *parser.SymTable) string {
	var sb strings.Builder
	for i := 0; i < len(ins); i++ {
		opcode := code.Opcode(ins[i])
		def, ok := code.Definitions[opcode]
		if ok {
			operand, offset := code.ReadOperand(def, ins[i+1:])
			format := strings.Repeat(" ", start) + " %-" + strconv.Itoa(indent) + "d %-22s %s"
			args := b.getArgs(def, operand, scope)
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
	s := parser.Search("Base", b.Symbols)
	return b.InsToString(b.Instruction, 1, 4, s)
}

func (b *Bytecode) getArgs(def code.Definition, operand []int, scope *parser.SymTable) string {
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
	case "OpSetLocal", "OpGetLocal", "OpUpdateLocal", "OpReturn", "OpReturnVal":
		if name, ok := scope.FindByIdx(idx); ok {
			args = name
		}
	case "OpLoadMethod":
		name := scope.Methods.FindName(idx)
		args += "(" + strconv.Quote(name) + ")"
	case "OpClosure":
		fn := b.Constants[idx].(object.CompiledFunc)
		var argSb strings.Builder
		argSb.WriteString("Call Func <" + fn.FnName + ">")
		return argSb.String()
	case "OpGetBuiltin":
		builtIn := object.BuiltinFns[idx]
		args = builtIn.Name
	case "OpSetGlobal", "OpGetGlobal", "OpUpdateGlobal":
		base := parser.Search("Base", b.Symbols)
		if name, ok := base.FindByIdx(idx); ok {
			args = name
		}
	}
	return args
}

func (b *Bytecode) decodeFunc() string {
	var sb strings.Builder
	for _, obj := range b.Constants {
		if obj.Type() == object.CompiledFuncObj {
			funcObj := obj.(object.CompiledFunc)
			sb.WriteString(fmt.Sprintf(`Disassembly of <FunctionObject %s at line %d>:`,
				funcObj.FnName, funcObj.LineLoc))
			sb.WriteString("\n")
			s := parser.Search(funcObj.FnName, b.Symbols)
			sb.WriteString(b.InsToString(funcObj.Instructions, 1, 4, s))
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}
