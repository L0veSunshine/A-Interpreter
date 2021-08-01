package vm

import (
	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/object"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	StackSize  = 2048
	GlobalSize = 65536
	MaxFrame   = 1024
)

var (
	StackOverErr = fmt.Errorf("stack overflow")
	StackIdxErr  = fmt.Errorf("invalid index")
	NullObj      = object.Null{}
)

type VM struct {
	constants []object.Object

	stack    [StackSize]object.Object
	globals  []object.Object
	sp       int
	frames   []Frame
	frameIdx int
}

func NewVM() *VM {
	frames := make([]Frame, MaxFrame)
	return &VM{
		globals:  make([]object.Object, GlobalSize),
		sp:       0, //stack pointer
		frames:   frames,
		frameIdx: 1,
	}
}

func (vm *VM) LastPop() object.Object {
	if vm.sp >= 0 {
		return vm.stack[vm.sp]
	}
	return NullObj
}

func (vm *VM) push(obj object.Object) error {
	nIdx := vm.sp + 1
	if nIdx > StackSize {
		return StackOverErr
	}
	vm.stack[vm.sp] = obj
	vm.sp = nIdx
	return nil
}

func (vm *VM) pop() object.Object {
	nIdx := vm.sp - 1
	obj := vm.stack[nIdx]
	vm.sp = nIdx
	return obj
}

func (vm *VM) top() object.Object {
	idx := vm.sp - 1
	if idx >= 0 {
		return vm.stack[idx]
	}
	return nil
}

func (vm *VM) replace(obj object.Object) error {
	idx := vm.sp - 1
	if idx >= 0 {
		vm.stack[idx] = obj
		return nil
	}
	return StackIdxErr
}

func (vm *VM) Run(bytecode *bytecode.Bytecode) error {
	var ip int
	var ins code.Instructions
	var op code.Opcode

	vm.frames[0] = NewFrame(bytecode.Instruction, vm.globals, 0)
	vm.constants = bytecode.Constants

	for vm.currentFrame().ip < len(vm.currentFrame().Instructions())-1 {
		vm.currentFrame().ip++
		ip = vm.currentFrame().ip
		ins = vm.currentFrame().Instructions()
		op = code.Opcode(ins[ip])
		switch op {
		case code.OpConstant:
			constIdx := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			err := vm.push(vm.constants[constIdx])
			if err != nil {
				return err
			}
		case code.OpTrue, code.OpFalse:
			if op == code.OpTrue {
				err := vm.push(nativeBoolToBool(true))
				if err != nil {
					return err
				}
			} else {
				err := vm.push(nativeBoolToBool(false))
				if err != nil {
					return err
				}
			}
		case code.OpPop:
			vm.pop()
		case code.OpPrintTop:
			vm.printTop()
		case code.OpAdd, code.OpSub, code.OpMul, code.OpDiv, code.OpPow, code.OpMod:
			err := vm.executeBinOp(op)
			if err != nil {
				return err
			}
		case code.OpPlus, code.OpMinus, code.OpNot:
			err := vm.executePrefix(op)
			if err != nil {
				return err
			}
		case code.OpEqual, code.OpNotEQ, code.OpGTEq, code.OpGT:
			err := vm.compareBinOp(op)
			if err != nil {
				return err
			}
		case code.OpAnd, code.OpOr:
			err := vm.logicBinOp(op)
			if err != nil {
				return err
			}
		case code.OpNull:
			err := vm.push(NullObj)
			if err != nil {
				return err
			}
		case code.OpJump:
			pos := int(code.ReadUint16(ins[ip+1:]))
			vm.currentFrame().ip = pos - 1
		case code.OpJumpNotTrue:
			pos := int(code.ReadUint16(ins[ip+1:]))
			vm.currentFrame().ip += 2 //skip the operand of code.OpJumpNotTrue
			cond := vm.pop()
			boolVal := objToNativeBool(cond)
			if !boolVal {
				vm.currentFrame().ip = pos - 1
			}
		case code.OpSetGlobal:
			globalIdx := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2 //skip the operand of code.OpSetGlobal
			vm.frames[0].vars[globalIdx] = vm.pop()
		case code.OpGetGlobal:
			globalIdx := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2 //skip the operand of code.OpGetGlobal
			err := vm.push(vm.frames[0].vars[globalIdx])
			if err != nil {
				return err
			}
		case code.OpUpdateGlobal:
			globalIdx := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2 //skip the operand of code.OpUpdate
			vm.frames[0].vars[globalIdx] = vm.top()
			if vm.sp > 1 {
				vm.sp--
			}
		case code.OpCallFunc:
			numArgs := code.ReadUint8(ins[ip+1:])
			vm.currentFrame().ip++
			err := vm.executeCall(int(numArgs))
			if err != nil {
				return err
			}
		case code.OpReturnVal:
			returnVal := vm.pop()

			frame := vm.popFrame()
			vm.sp = frame.basePoint - 1

			err := vm.push(returnVal)
			if err != nil {
				return err
			}
		case code.OpReturn:
			frame := vm.popFrame()
			vm.sp = frame.basePoint - 1
			err := vm.push(object.Null{})
			if err != nil {
				return err
			}
		case code.OpSetLocal:
			localIndex := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			vm.currentFrame().vars[localIndex] = vm.pop()
		case code.OpGetLocal:
			localIndex := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			err := vm.push(vm.currentFrame().vars[localIndex])
			if err != nil {
				return err
			}
		case code.OpUpdateLocal:
			localIndex := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			vm.currentFrame().vars[localIndex] = vm.top()
			if vm.sp > 1 {
				vm.sp--
			}
		case code.OpGetBuiltin:
			builtinIdx := code.ReadUint8(ins[ip+1:])
			vm.currentFrame().ip += 1
			def := object.BuiltinFns[builtinIdx]
			err := vm.push(def.Builtin)
			if err != nil {
				return err
			}
		case code.OpClosure:
			fnIdx := code.ReadUint8(ins[ip+1:])
			vm.currentFrame().ip += 1
			err := vm.push(vm.constants[fnIdx])
			if err != nil {
				return err
			}
		case code.OpBuildArray:
			gap := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			var ele []object.Object
			for i := vm.sp - int(gap); i < vm.sp; i++ {
				ele = append(ele, vm.stack[i])
			}
			vm.sp -= int(gap) //栈指针归位
			err := vm.push(object.Array{Elements: ele})
			if err != nil {
				return err
			}
		case code.OpMakeSlice:
			sliceObj := object.Slice{}
			sliceObj.Start = vm.stack[vm.sp-3]
			sliceObj.End = vm.stack[vm.sp-2]
			sliceObj.Step = vm.stack[vm.sp-1]
			vm.sp -= 3
			err := vm.push(sliceObj)
			if err != nil {
				return err
			}
		case code.OpIndexArray:
			err := vm.applyIndex()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (vm *VM) executeBinOp(op code.Opcode) error {
	right := vm.pop()
	left := vm.top()
	switch {
	case right.Type() == object.IntObj && left.Type() == object.IntObj:
		return vm.executeBinOpInt(op, left, right)
	case right.Type() == object.StringObj || left.Type() == object.StringObj:
		return vm.executeBinOpStr(op, left, right)
	case right.Type() == object.FloatObj || left.Type() == object.FloatObj:
		return vm.executeBinOpFloat(op, left, right)
	}
	return fmt.Errorf("unsupported types for binary operation: %s(%s) %s(%s) %s",
		left.Type(), left.Inspect(), right.Type(), right.Inspect(), code.Definitions[op].Name)
}

func (vm *VM) compareBinOp(op code.Opcode) error {
	right := vm.pop()
	left := vm.top()
	if (left.Type() == object.IntObj || left.Type() == object.FloatObj) &&
		(right.Type() == object.IntObj || right.Type() == object.FloatObj) {
		return vm.compareNumObj(op, left, right)
	}
	var res bool
	switch op {
	case code.OpEqual:
		res = left == right
	case code.OpNotEQ:
		res = left != right
	default:
		return fmt.Errorf("unsupport operator for object: %d", op)
	}
	return vm.replace(nativeBoolToBool(res))
}

func (vm *VM) logicBinOp(op code.Opcode) error {
	right := vm.pop()
	left := vm.top()
	switch {
	case left.Type() == object.BooleanObj && right.Type() == object.BooleanObj:
		var res bool
		leftVal := left.(object.Boolean).Value
		rightVal := right.(object.Boolean).Value
		switch op {
		case code.OpAnd:
			res = leftVal && rightVal
		case code.OpOr:
			res = leftVal || rightVal
		}
		return vm.replace(nativeBoolToBool(res))
	default:
		return fmt.Errorf("unsporrted type %s", right.Type())
	}
}

func (vm *VM) compareNumObj(op code.Opcode, left, right object.Object) error {
	var leftVal float64
	var rightVal float64
	var res bool
	switch left.(type) {
	case object.Int:
		leftVal = float64(left.(object.Int).Value)
	case object.Float:
		leftVal = left.(object.Float).Value
	}
	switch right.(type) {
	case object.Int:
		rightVal = float64(right.(object.Int).Value)
	case object.Float:
		rightVal = right.(object.Float).Value
	}
	switch op {
	case code.OpEqual:
		res = leftVal == rightVal
	case code.OpNotEQ:
		res = leftVal != rightVal
	case code.OpGT:
		res = leftVal > rightVal
	case code.OpGTEq:
		res = leftVal >= rightVal
	default:
		return fmt.Errorf("unsupport operator for NumberObj: %d", op)
	}
	return vm.replace(nativeBoolToBool(res))
}

func nativeBoolToBool(b bool) object.Boolean {
	return object.Boolean{Value: b}
}

func objToNativeBool(obj object.Object) bool {
	switch obj := obj.(type) {
	case object.Boolean:
		if obj.Value {
			return true
		}
		return false
	case object.Int:
		if obj.Value == 0 {
			return false
		}
		return true
	case object.Float:
		if obj.Value == 0 {
			return false
		}
		return true
	default:
		return true
	}
}

func (vm *VM) executeBinOpInt(op code.Opcode, left, right object.Object) error {
	leftVal := left.(object.Int).Value
	rightVal := right.(object.Int).Value
	var res int
	var fRes float64
	switch op {
	case code.OpAdd:
		res = leftVal + rightVal
	case code.OpSub:
		res = leftVal - rightVal
	case code.OpMul:
		res = leftVal * rightVal
	case code.OpDiv:
		if rightVal != 0 {
			fRes = float64(leftVal) / float64(rightVal)
		} else {
			return errors.New("division by zero")
		}
		return vm.replace(object.Float{Value: fRes})
	case code.OpMod:
		res = leftVal % rightVal
	case code.OpPow:
		fRes = math.Pow(float64(leftVal), float64(rightVal))
		return vm.replace(object.Float{Value: fRes})
	default:
		return fmt.Errorf("unknown integer operator: %d", op)
	}
	return vm.replace(object.Int{Value: res})
}

func (vm *VM) executeBinOpFloat(op code.Opcode, left, right object.Object) error {
	var leftVal float64
	var rightVal float64
	switch left.(type) {
	case object.Int:
		leftVal = float64(left.(object.Int).Value)
	case object.Float:
		leftVal = left.(object.Float).Value
	}
	switch right.(type) {
	case object.Int:
		rightVal = float64(right.(object.Int).Value)
	case object.Float:
		rightVal = right.(object.Float).Value
	}
	var res float64
	switch op {
	case code.OpAdd:
		res = leftVal + rightVal
	case code.OpSub:
		res = leftVal - rightVal
	case code.OpMul:
		res = leftVal * rightVal
	case code.OpDiv:
		if rightVal != 0 {
			res = leftVal / rightVal
		} else {
			return errors.New("division by zero")
		}
	case code.OpMod:
		res = math.Mod(leftVal, rightVal)
	case code.OpPow:
		res = math.Pow(leftVal, rightVal)
	default:
		return fmt.Errorf("unknown operator %d for integer", op)
	}
	return vm.replace(object.Float{Value: res})
}

func (vm *VM) executeBinOpStr(op code.Opcode, left, right object.Object) error {
	var leftVal, rightVal string
	leftVal = left.Inspect()
	rightVal = right.Inspect()
	if op == code.OpAdd {
		return vm.replace(object.String{Value: []rune(leftVal + rightVal)})
	} else {
		return fmt.Errorf("unknown operator %d for str", op)
	}
}

func (vm *VM) executePrefix(op code.Opcode) error {
	token := vm.top()
	switch token.Type() {
	case object.IntObj:
		value := token.(object.Int).Value
		switch op {
		case code.OpMinus:
			value = -value
		case code.OpPlus:
		default:
			return fmt.Errorf("unkonwn opCode %s for %s", string(op), token.Type())
		}
		return vm.replace(object.Int{Value: value})
	case object.FloatObj:
		value := token.(object.Float).Value
		switch op {
		case code.OpMinus:
			value = -value
		case code.OpPlus:
		default:
			return fmt.Errorf("unkonwn opCode %s for %s", string(op), token.Type())
		}
		return vm.replace(object.Float{Value: value})
	case object.BooleanObj:
		value := token.(object.Boolean).Value
		if op == code.OpNot {
			value = !value
		} else {
			return fmt.Errorf("unkonwn opCode %s for %s", string(op), token.Type())
		}
		return vm.replace(nativeBoolToBool(value))
	}
	return fmt.Errorf("unsupported type for negation: %s", token.Type())
}

func (vm *VM) applyIndex() error {
	ident := vm.pop()
	switch ident.Type() {
	case object.IntObj:
		return vm.integerIndex(ident.(object.Int).Value)
	case object.SliceObj:
		return vm.sliceIndex(ident.(object.Slice))
	}
	return fmt.Errorf("unsupported type for index: %s", ident.Type())
}

func (vm *VM) integerIndex(idx int) error {
	obj := vm.top()
	switch obj := obj.(type) {
	case object.Array:
		maxLen := len(obj.Elements) - 1
		if idx > maxLen {
			return fmt.Errorf("%s index out of range", obj.Type())
		} else if idx < 0 {
			idx = maxLen + 1 + idx
			if idx < 0 {
				idx = 0
			}
		}
		err := vm.replace(obj.Elements[idx])
		if err != nil {
			return err
		}
	case object.String:
		maxLen := len(obj.Value) - 1
		if idx > maxLen {
			idx = maxLen
		} else if idx < 0 {
			idx = maxLen + 1 + idx
			if idx < 0 {
				idx = 0
			}
		}
		err := vm.replace(object.String{Value: []rune{obj.Value[idx]}})
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("%s object is not subscriptable", obj.Type())
	}
	return nil
}

func (vm *VM) sliceIndex(s object.Slice) error {
	obj := vm.top()
	var start, end, step int
	switch obj := obj.(type) {
	case object.Array:
		start, end, step = vm.handleSlice(s, len(obj.Elements))
		var newArr []object.Object
		if step > 0 {
			for ; start < end; start += step {
				newArr = append(newArr, obj.Elements[start])
			}
		} else {
			for ; start > end; start += step {
				newArr = append(newArr, obj.Elements[start])
			}
		}
		err := vm.replace(object.Array{Elements: newArr})
		if err != nil {
			return err
		}
	case object.String:
		start, end, step = vm.handleSlice(s, len(obj.Value))
		var newStr []rune
		if step > 0 {
			for ; start < end; start += step {
				newStr = append(newStr, obj.Value[start])
			}
		} else {
			for ; start > end; start += step {
				newStr = append(newStr, obj.Value[start])
			}
		}
		err := vm.replace(object.String{Value: newStr})
		if err != nil {
			return err
		}
	}
	return nil
}

func (vm *VM) handleSlice(s object.Slice, maxLen int) (start, end, step int) {
	if s.Start.Type() != object.NullObj {
		start = s.Start.(object.Int).Value
	} else {
		start = 0
	}
	if s.End.Type() != object.NullObj {
		end = s.End.(object.Int).Value
		if end < 0 {
			end = maxLen + 1 + end
		}
	} else {
		end = maxLen
	}
	if s.Step.Type() != object.NullObj {
		step = s.Step.(object.Int).Value
		if step < 0 && end == maxLen && start == 0 {
			start = maxLen - 1
			end = -1
		}
	} else {
		step = 1
	}
	return
}

func (vm *VM) currentFrame() *Frame {
	return &vm.frames[vm.frameIdx-1]
}

func (vm *VM) pushFrame(frame Frame) {
	vm.frames[vm.frameIdx] = frame
	vm.frameIdx++
}

func (vm *VM) popFrame() Frame {
	vm.frameIdx--
	return vm.frames[vm.frameIdx]
}

var callee object.Object

func (vm *VM) executeCall(numArgs int) error {
	callee = vm.stack[vm.sp-1-numArgs]
	switch callee := callee.(type) {
	case object.CompiledFunc:
		return vm.callFunc(callee, numArgs)
	case object.Builtin:
		return vm.callBuiltin(callee, numArgs)
	default:
		return fmt.Errorf("calling non-function and non-built-in")
	}
}

func (vm *VM) printTop() {
	topObj := vm.top()
	if topObj != NullObj {
		printFn := object.GetBuiltinFn("print")
		printFn.Fn(topObj)
		vm.sp = vm.sp - 2
	}
}

var frame Frame

func (vm *VM) callFunc(fn object.CompiledFunc, numArgs int) error {
	if numArgs != fn.ParametersNum {
		return fmt.Errorf("wrong number of arguments: want=%d, got=%d",
			fn.ParametersNum, numArgs)
	}
	newVars := make([]object.Object, fn.LocalsNum)
	frame = NewFrame(fn.Instructions, newVars, vm.sp-numArgs)
	fnArgs := vm.stack[vm.sp-numArgs : vm.sp]
	for idx, arg := range fnArgs {
		frame.vars[idx] = arg
	}
	vm.pushFrame(frame)
	return nil
}

var args []object.Object

func (vm *VM) callBuiltin(builtin object.Builtin, argNums int) error {
	args = vm.stack[vm.sp-argNums : vm.sp]
	result := builtin.Fn(args...)
	vm.sp = vm.sp - argNums - 1
	if result != nil {
		return vm.push(result)
	}
	return vm.push(NullObj)
}

func (vm *VM) debug(operandWidth int) {
	var sb strings.Builder
	sb.WriteString("[")
	for idx, o := range vm.stack {
		if o != nil {
			sb.WriteString(o.Inspect() + ",")
		} else {
			sb.WriteString("nil,")
		}
		if idx == vm.sp {
			break
		}
	}
	sb.WriteString("]")
	fmt.Println("Current Frame:" + strconv.Itoa(vm.frameIdx) + " Instruction Pointer:" +
		strconv.Itoa(vm.currentFrame().ip-operandWidth))
	fmt.Println("Stack: "+sb.String()+"  Current Pointer:", vm.sp)
}
