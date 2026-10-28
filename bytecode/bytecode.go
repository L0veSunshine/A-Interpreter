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
	// Lines 是主函数的"指令偏移 -> 源码行"映射
	Lines     []code.LineEntry
	Constants []object.Object
	Symbols   *parser.SymTable
	// MainLocals 是顶层作用域（含顶层块）需要的局部槽位数
	MainLocals int
	// MainMaxStack 是主函数运行期需要的操作数栈上界。
	// VM 在建帧时按它一次性预留，循环里就不必逐条指令检查容量。
	// 0 表示"未知"，此时 VM 会退回到保守预留。
	MainMaxStack int
}

func (b *Bytecode) Ins() string {
	ls := strings.Repeat("=", 50) + "\n"
	var sb strings.Builder
	sb.WriteString(ls)
	sb.WriteString(b.decodeFunc())

	sb.WriteString(fmt.Sprintf("%30s\n", "<Main Function>"))
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
	return b.InsToString(b.Instruction, 1, 6, s)
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
	case "OpSetLocal", "OpGetLocal", "OpUpdateLocal":
		if name, ok := scope.FindLocalByIdx(idx); ok {
			args = name
		}
	case "OpLoadMethod":
		name := scope.Methods.FindName(idx)
		args += "(" + strconv.Quote(name) + ")"
	case "OpClosure":
		fn := b.Constants[idx].(object.CompiledFunc)
		var argSb strings.Builder
		argSb.WriteString("Get Func <" + fn.FnName + ">")
		return argSb.String()
	case "OpGetBuiltin":
		builtIn := object.BuiltinFns[idx]
		args = builtIn.Name
	case "OpSetGlobal", "OpGetGlobal", "OpUpdateGlobal":
		base := parser.Search("Base", b.Symbols)
		if name, ok := base.FindGlobalByIdx(idx); ok {
			args = name
		}
	case "OpBinFused":
		return b.fusedBinArgs(operand, scope)
	case "OpReturnOperand":
		return "return " + b.operandName(byte(operand[0]), operand[1], scope)
	case "OpBinReturn":
		return "return " + opName(code.Opcode(operand[0]))
	}
	return args
}

// opName 返回操作码的可读名字；未定义时退化成数字。
func opName(op code.Opcode) string {
	if def, ok := code.Definitions[op]; ok {
		return def.Name
	}
	return "opcode[" + strconv.Itoa(int(op)) + "]"
}

// operandName 把"来源 + 下标"渲染成可读形式，便于看反汇编。
func (b *Bytecode) operandName(kind byte, idx int, scope *parser.SymTable) string {
	switch kind {
	case code.OperandLocal:
		if name, ok := scope.FindLocalByIdx(idx); ok {
			return name
		}
		return "local[" + strconv.Itoa(idx) + "]"
	case code.OperandGlobal:
		base := parser.Search("Base", b.Symbols)
		if name, ok := base.FindGlobalByIdx(idx); ok {
			return name
		}
		return "global[" + strconv.Itoa(idx) + "]"
	default:
		if idx < len(b.Constants) {
			obj := b.Constants[idx]
			return string(obj.Type()) + "(" + obj.Inspect() + ")"
		}
		return "const[" + strconv.Itoa(idx) + "]"
	}
}

// fusedBinArgs 把 OpBinFused 渲染成「左边 运算符 右边」。
func (b *Bytecode) fusedBinArgs(operand []int, scope *parser.SymTable) string {
	if len(operand) != 4 {
		return ""
	}
	kindAB := byte(operand[0])
	subOp := code.Opcode(operand[1])
	return b.operandName(kindAB>>4, operand[2], scope) + " " +
		opName(subOp) + " " + b.operandName(kindAB&0x0f, operand[3], scope)
}

func (b *Bytecode) decodeFunc() string {
	var sb strings.Builder
	for _, obj := range b.Constants {
		if obj.Type() == object.CompiledFuncObj {
			funcObj := obj.(object.CompiledFunc)
			sb.WriteString(fmt.Sprintf(`Disassembly of <FunctionObject "%s" at line %d>:`,
				funcObj.FnName, funcObj.LineLoc))
			sb.WriteString("\n")
			s := parser.Search(funcObj.FnName, b.Symbols)
			if funcObj.ScopeID != 0 {
				if scoped := b.Symbols.FindScope(funcObj.ScopeID); scoped != nil {
					s = scoped
				}
			}
			sb.WriteString(b.InsToString(funcObj.Instructions, 1, 6, s))
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}
