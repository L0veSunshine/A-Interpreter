package vm

import (
	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/format"
	"Interpreter/object"
	"Interpreter/utils"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	// InitStackSize 是操作数栈的初始槽位数；push 时会按需翻倍，
	// 直到 MaxStackSize。原来固定 2048 槽，深递归到 680 层左右就爆栈。
	InitStackSize = 2048
	MaxStackSize  = 1 << 20
	// InitGlobalSize 是全局槽位的初始容量，按需增长。
	// 槽位号来自 2 字节操作数，所以真正的上限是 65536，
	// 由编译器在越界时报错（见 compiler.emit），VM 这里只兜底。
	InitGlobalSize = 256
	MaxGlobalSize  = 1 << 16
	// InitFrameCount 是调用栈的初始容量，同样按需增长，上限 MaxFrame
	InitFrameCount = 64
	MaxFrame       = 1 << 16
)

var (
	StackIdxErr = fmt.Errorf(format.Alert + "invalid index")
	NullObj     = object.Null{}
)

type VM struct {
	constants []object.Object

	stack    []object.Object
	globals  []object.Object
	sp       int
	maxSP    int
	frames   []Frame
	frameIdx int
	// lastPop 记录"最后一条语句级表达式产生的值"，供 REPL / 测试读取。
	// 之前靠"值留在栈顶"实现，正是循环泄漏栈槽的根源。
	lastPop object.Object
	// argBuf 是调用内建函数/方法时的实参缓冲。做成实例字段，
	// 避免两个 VM 共享同一个包级切片而串数据。
	argBuf []object.Object
}

func NewVM() *VM {
	return &VM{
		stack:    make([]object.Object, InitStackSize),
		globals:  make([]object.Object, InitGlobalSize),
		sp:       0, //stack pointer
		frames:   make([]Frame, InitFrameCount),
		frameIdx: 1,
	}
}

func (vm *VM) LastPop() object.Object {
	if vm.lastPop != nil {
		return vm.lastPop
	}
	if vm.sp > 0 && vm.sp <= len(vm.stack) {
		return vm.stack[vm.sp-1]
	}
	return NullObj
}

// MaxStackDepth 返回本次执行中操作数栈达到过的最大高度，
// 用来验证"循环不再每轮泄漏一个栈槽"。
func (vm *VM) MaxStackDepth() int {
	return vm.maxSP
}

func (vm *VM) push(obj object.Object) error {
	if vm.sp == len(vm.stack) {
		if len(vm.stack) >= MaxStackSize {
			f := vm.currentFrame()
			return fmt.Errorf("%sstack overflow: operand stack exceeded %d slots in %s (line %d)",
				format.Alert, MaxStackSize, f.Name, f.Line)
		}
		grown := len(vm.stack) * 2
		if grown > MaxStackSize {
			grown = MaxStackSize
		}
		newStack := make([]object.Object, grown)
		copy(newStack, vm.stack)
		vm.stack = newStack
	}
	vm.stack[vm.sp] = obj
	vm.sp++
	if vm.sp > vm.maxSP {
		vm.maxSP = vm.sp
	}
	return nil
}

func (vm *VM) pop() object.Object {
	if vm.sp <= 0 {
		vm.sp = 0
		return NullObj
	}
	vm.sp--
	return vm.stack[vm.sp]
}

func (vm *VM) top() object.Object {
	idx := vm.sp - 1
	if idx >= 0 {
		return vm.stack[idx]
	}
	// 空栈时返回 None，让后续的类型检查给出正常错误而不是空指针崩溃
	return NullObj
}

func (vm *VM) replace(obj object.Object) error {
	idx := vm.sp - 1
	if idx >= 0 {
		vm.stack[idx] = obj
		return nil
	}
	return StackIdxErr
}

func (vm *VM) Run(bytecode *bytecode.Bytecode) (err error) {
	var ip int
	var ins code.Instructions
	var op code.Opcode
	var varIdx uint16

	mainVars := make([]object.Object, bytecode.MainLocals)
	for i := range mainVars {
		mainVars[i] = NullObj
	}
	// main 帧拥有独立的局部槽位；全局变量单独放在 vm.globals 里
	vm.frames[0] = NewFrame(bytecode.Instruction, &mainVars, 0).WithLines(bytecode.Lines)
	vm.frames[0].Name = "<main>"
	vm.constants = bytecode.Constants

	// 统一的错误定位：任何从指令执行里冒出来的错误都补上源码行，
	// 这样调用方拿到的错误信息能直接指到源码（不需要逐处 return 时处理）
	defer func() {
		if err != nil {
			err = vm.locate(err)
		}
	}()

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
			vm.lastPop = vm.pop()
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
		case code.OpEqual, code.OpNotEQ, code.OpGTEq, code.OpGT, code.OpLT, code.OpLTEq:
			err := vm.compareBinOp(op)
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
			varIdx = code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2 //skip the operand of code.OpSetGlobal
			if err := vm.setGlobal(varIdx, vm.pop()); err != nil {
				return err
			}
		case code.OpGetGlobal:
			varIdx = code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2 //skip the operand of code.OpGetGlobal
			err := vm.push(vm.globalAt(varIdx))
			if err != nil {
				return err
			}
		case code.OpUpdateGlobal:
			varIdx = code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2 //skip the operand of code.OpUpdate
			if err := vm.setGlobal(varIdx, vm.top()); err != nil {
				return err
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
			if vm.frameIdx <= 1 {
				return fmt.Errorf(format.Alert + "return outside of a function")
			}

			frame := vm.popFrame()
			vm.sp = frame.basePoint - 1
			if vm.sp < 0 {
				vm.sp = 0
			}

			err := vm.push(returnVal)
			if err != nil {
				return err
			}
		case code.OpSetLocal:
			varIdx = code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			vm.currentFrame().vars[varIdx] = vm.pop()
		case code.OpGetLocal:
			varIdx = code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			err := vm.push(vm.localAt(varIdx))
			if err != nil {
				return err
			}
		case code.OpUpdateLocal:
			varIdx = code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			vm.currentFrame().vars[varIdx] = vm.top()
		case code.OpGetBuiltin:
			builtinIdx := code.ReadUint8(ins[ip+1:])
			vm.currentFrame().ip += 1
			err := vm.push(object.BuiltinFns[builtinIdx].Builtin)
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
		case code.OpIndex:
			err := vm.applyIndex()
			if err != nil {
				return err
			}
		case code.OpUpdate:
			err := vm.arrayUpdate()
			if err != nil {
				return err
			}
		case code.OpMakeMap:
			keyLen := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			err := vm.makeMap(int(keyLen))
			if err != nil {
				return err
			}
		case code.OpLoadMethod:
			varIdx = code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			mName := bytecode.Symbols.Methods.FindName(int(varIdx))
			objType := vm.top().Type()
			var method object.Object
			var err error
			method, err = object.FindMethod(objType, mName)
			if err != nil {
				return err
			}
			err = vm.push(method)
			if err != nil {
				return err
			}
		case code.OpCallMethod:
			argsNum := code.ReadUint8(ins[ip+1:])
			vm.currentFrame().ip += 1
			err := vm.callMethod(int(argsNum))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (vm *VM) callMethod(argsNum int) error {
	baseIdx := vm.sp - argsNum - 1
	if baseIdx-1 < 0 {
		return fmt.Errorf(format.Alert + "invalid method call: not enough values on stack")
	}
	obj := vm.stack[baseIdx-1]
	method := vm.stack[baseIdx]
	baseIdx++
	vm.argBuf = vm.argBuf[:0]
	for ; baseIdx < vm.sp; baseIdx++ {
		vm.argBuf = append(vm.argBuf, vm.stack[baseIdx])
	}
	methodFunc, ok := method.(object.MethodObj)
	if !ok || methodFunc.M == nil {
		return fmt.Errorf(format.Alert + "Method is invaild")
	}
	returnObj := methodFunc.M(obj, vm.argBuf...)
	vm.sp = vm.sp - argsNum - 2
	var err error
	if len(returnObj) >= 2 {
		err = vm.push(returnObj[1]) //push returned value
	}
	err = vm.push(returnObj[0]) //push self
	if err != nil {
		return err
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
	return fmt.Errorf(format.Alert+"unsupported types for binary operation: %s(%s) %s(%s) %s",
		left.Type(), left.Inspect(), right.Type(), right.Inspect(), code.Definitions[op].Name)
}

func (vm *VM) compareBinOp(op code.Opcode) error {
	right := vm.pop()
	left := vm.top()
	if (left.Type() == object.IntObj || left.Type() == object.FloatObj) &&
		(right.Type() == object.IntObj || right.Type() == object.FloatObj) {
		return vm.compareNumObj(op, left, right)
	}
	if left.Type() == object.StringObj && right.Type() == object.StringObj {
		return vm.compareStringObj(op, left, right)
	}
	return vm.compareObj(op, left, right)
}

func (vm *VM) compareObj(op code.Opcode, left, right object.Object) error {
	// 不能用哈希值比较：Array/Map 等复合类型没有稳定的哈希语义
	eq := object.RichCompare(left, right)
	var res bool
	switch op {
	case code.OpEqual:
		res = eq
	case code.OpNotEQ:
		res = !eq
	default:
		return fmt.Errorf(format.Alert+"unsupport operator for object(%s,%s): %d", left.Inspect(),
			right.Inspect(), op)
	}
	return vm.replace(nativeBoolToBool(res))
}

func (vm *VM) compareStringObj(op code.Opcode, left, right object.Object) error {
	var leftVal = string(left.(object.String).Value)
	var rightVal = string(right.(object.String).Value)
	var res bool
	switch op {
	case code.OpEqual:
		res = leftVal == rightVal
	case code.OpNotEQ:
		res = leftVal != rightVal
	case code.OpGT:
		res = leftVal > rightVal
	case code.OpGTEq:
		res = leftVal >= rightVal
	case code.OpLT:
		res = leftVal < rightVal
	case code.OpLTEq:
		res = leftVal <= rightVal
	default:
		return fmt.Errorf(format.Alert+"unsupport operator for StringObj: %d", op)
	}
	return vm.replace(nativeBoolToBool(res))
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
	case code.OpLT:
		res = leftVal < rightVal
	case code.OpLTEq:
		res = leftVal <= rightVal
	default:
		return fmt.Errorf(format.Alert+"unsupport operator for NumberObj: %d", op)
	}
	return vm.replace(nativeBoolToBool(res))
}

func nativeBoolToBool(b bool) object.Boolean {
	return object.Boolean{Value: b}
}

func objToNativeBool(obj object.Object) bool {
	switch obj := obj.(type) {
	case object.Boolean:
		return obj.Value
	case object.Int:
		return obj.Value != 0
	case object.Float:
		return obj.Value != 0
	case object.Null:
		return false
	case object.String:
		return len(obj.Value) > 0
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
			return DivZeroErr
		}
		return vm.replace(object.Float{Value: fRes})
	case code.OpMod:
		if rightVal == 0 {
			return DivZeroErr
		}
		res = leftVal % rightVal
	case code.OpPow:
		fRes = math.Pow(float64(leftVal), float64(rightVal))
		return vm.replace(object.Float{Value: fRes})
	default:
		return fmt.Errorf(format.Alert+"unknown integer operator: %d", op)
	}
	return vm.replace(object.Int{Value: res})
}

var DivZeroErr = fmt.Errorf(format.Alert + "division by zero")

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
			return DivZeroErr
		}
	case code.OpMod:
		res = math.Mod(leftVal, rightVal)
	case code.OpPow:
		res = math.Pow(leftVal, rightVal)
	default:
		return fmt.Errorf(format.Alert+"unknown operator %d for integer", op)
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
		return fmt.Errorf(format.Alert+"unknown operator %d for str", op)
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
			return fmt.Errorf(format.Alert+"unkonwn opCode %s for %s", string(op), token.Type())
		}
		return vm.replace(object.Int{Value: value})
	case object.FloatObj:
		value := token.(object.Float).Value
		switch op {
		case code.OpMinus:
			value = -value
		case code.OpPlus:
		default:
			return fmt.Errorf(format.Alert+"unkonwn opCode %s for %s", string(op), token.Type())
		}
		return vm.replace(object.Float{Value: value})
	case object.BooleanObj:
		value := token.(object.Boolean).Value
		if op == code.OpNot {
			value = !value
		} else {
			return fmt.Errorf(format.Alert+"unkonwn opCode %s for %s", string(op), token.Type())
		}
		return vm.replace(nativeBoolToBool(value))
	case object.NullObj:
		// None 是假值：not None == True；+None / -None 仍然是类型错误
		if op == code.OpNot {
			return vm.replace(nativeBoolToBool(true))
		}
		return fmt.Errorf(format.Alert+"unsupported type for negation: %s", token.Type())
	}
	return fmt.Errorf(format.Alert+"unsupported type for negation: %s", token.Type())
}

func (vm *VM) applyIndex() error {
	IdxIdent := vm.pop()
	switch IdxIdent.Type() {
	case object.IntObj:
		return vm.integerIndex(IdxIdent.(object.Int).Value)
	case object.SliceObj:
		return vm.sliceIndex(IdxIdent.(object.Slice))
	case object.StringObj, object.FloatObj, object.BooleanObj:
		return vm.mapIndex(utils.Hash(IdxIdent))
	}
	return fmt.Errorf(format.Alert+"unsupported type for index: %s", IdxIdent.Type())
}

func (vm *VM) integerIndex(idx int) error {
	obj := vm.top()
	var err error
	switch obj := obj.(type) {
	case object.Array:
		maxLen := len(obj.Elements) - 1
		if idx > maxLen {
			return fmt.Errorf(format.Alert+"%s index out of range", obj.Type())
		} else if idx < 0 {
			idx = maxLen + 1 + idx
			if idx < 0 {
				idx = 0
			}
		}
		err = vm.replace(obj.Elements[idx])
		if err != nil {
			return err
		}
	case object.String:
		maxLen := len(obj.Value) - 1
		if idx > maxLen {
			return fmt.Errorf(format.Alert+"%s index out of range", obj.Type())
		} else if idx < 0 {
			idx = maxLen + 1 + idx
			if idx < 0 {
				idx = 0
			}
		}
		err = vm.replace(object.String{Value: []rune{obj.Value[idx]}})
		if err != nil {
			return err
		}
	case object.Map:
		hash := idx + 193460240 //time33("IntObj")
		m, ok := obj.Store[hash]
		if !ok {
			err = vm.replace(NullObj)
		} else {
			err = vm.replace(m.Item)
		}
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf(format.Alert+"%s object is not subscriptable", obj.Type())
	}
	return nil
}

func (vm *VM) mapIndex(hash int) error {
	obj := vm.top()
	if obj.Type() != object.MapObj {
		return fmt.Errorf(format.Alert+"%s object is not subscriptable by a non-integer index", obj.Type())
	}
	mapObj := obj.(object.Map)
	m, ok := mapObj.Store[hash]
	var err error
	if !ok {
		err = vm.replace(NullObj)
	} else {
		err = vm.replace(m.Item)
	}
	if err != nil {
		return err
	}
	return nil
}

func (vm *VM) sliceIndex(s object.Slice) error {
	obj := vm.top()
	var start, end, step int
	var err error
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
		err = vm.replace(object.Array{Elements: newArr})
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
		err = vm.replace(object.String{Value: newStr})
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

func (vm *VM) arrayUpdate() error {
	key := vm.pop()
	array := vm.pop()
	var idxI int
	target := vm.pop()
	var pointer *object.Array
	switch array := array.(type) {
	case object.Array:
		idx, ok := key.(object.Int)
		if !ok {
			return fmt.Errorf(format.Alert+"error index %s", key.Inspect())
		}
		idxI = idx.Value
		length := len(array.Elements)
		if idxI < 0 {
			idxI = idxI + length
		}
		if idxI < 0 || idxI >= len(array.Elements) {
			return fmt.Errorf(format.Alert + "list assignment index out of range")
		}
		pointer = &array
		pointer.Elements[idxI] = target
		err := vm.push(array)
		if err != nil {
			return err
		}
	case object.Map:
		idxI = utils.Hash(key)
		p, ok := array.Store[idxI]
		if !ok {
			newPair := object.MapPair{
				Key:  key,
				Item: target,
			}
			array.Store[idxI] = newPair
			array.Size++
		} else {
			p.Item = target
			array.Store[idxI] = p
		}
		err := vm.push(array)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf(format.Alert+"type %s don't support setItem operation", array.Type())
	}
	return nil
}

func (vm *VM) makeMap(keyLen int) error {
	mapObj := object.Map{
		Store: map[int]object.MapPair{},
		Size:  keyLen / 2,
	}
	if keyLen == 0 {
		err := vm.push(mapObj)
		if err != nil {
			return err
		}
		return nil
	}
	idx := vm.sp - keyLen
	for idx < vm.sp {
		p := object.MapPair{
			Key:  vm.stack[idx],
			Item: vm.stack[idx+1],
		}
		hash := utils.Hash(p.Key)
		mapObj.Store[hash] = p
		idx += 2
	}
	vm.sp -= keyLen
	err := vm.push(mapObj)
	if err != nil {
		return err
	}
	return nil
}

func (vm *VM) currentFrame() *Frame {
	return &vm.frames[vm.frameIdx-1]
}

// globalAt/localAt 对"越界或尚未赋值"的槽位返回 None，
// 保证解释器不会因为编译器的一处疏漏而空指针崩溃。
func (vm *VM) globalAt(idx uint16) object.Object {
	if int(idx) >= len(vm.globals) || vm.globals[idx] == nil {
		return NullObj
	}
	return vm.globals[idx]
}

// setGlobal 按需扩容全局槽位表。上限 MaxGlobalSize 与 2 字节操作数宽度一致，
// 编译器会在符号号越界时先报错，这里只是兜底（手写字节码的情况）。
func (vm *VM) setGlobal(idx uint16, obj object.Object) error {
	if int(idx) >= MaxGlobalSize {
		return fmt.Errorf("%sglobal slot %d exceeds limit %d", format.Alert, idx, MaxGlobalSize)
	}
	if grow := int(idx) + 1; grow > len(vm.globals) {
		grown := len(vm.globals) * 2
		for grown < grow {
			grown *= 2
		}
		if grown > MaxGlobalSize {
			grown = MaxGlobalSize
		}
		newGlobals := make([]object.Object, grown)
		copy(newGlobals, vm.globals)
		vm.globals = newGlobals
	}
	vm.globals[idx] = obj
	return nil
}

func (vm *VM) localAt(idx uint16) object.Object {
	vars := vm.currentFrame().vars
	if int(idx) >= len(vars) || vars[idx] == nil {
		return NullObj
	}
	return vars[idx]
}

func (vm *VM) pushFrame(frame Frame) error {
	if vm.frameIdx >= MaxFrame {
		return fmt.Errorf("%scall stack overflow: call depth exceeded %d while calling %s (line %d)",
			format.Alert, MaxFrame, frame.Name, frame.Line)
	}
	if vm.frameIdx == len(vm.frames) {
		grown := len(vm.frames) * 2
		if grown > MaxFrame {
			grown = MaxFrame
		}
		newFrames := make([]Frame, grown)
		copy(newFrames, vm.frames)
		vm.frames = newFrames
	}
	vm.frames[vm.frameIdx] = frame
	vm.frameIdx++
	return nil
}

func (vm *VM) popFrame() Frame {
	vm.frameIdx--
	return vm.frames[vm.frameIdx]
}

func (vm *VM) executeCall(numArgs int) error {
	if vm.sp < numArgs+1 {
		return fmt.Errorf(format.Alert+"invalid call: stack has %d slots but call needs %d",
			vm.sp, numArgs+1)
	}
	callee := vm.stack[vm.sp-1-numArgs]
	switch callee := callee.(type) {
	case object.CompiledFunc:
		return vm.callFunc(callee, numArgs)
	case object.Builtin:
		return vm.callBuiltin(callee, numArgs)
	default:
		return fmt.Errorf(format.Alert+"calling non-function and non-built-in (type %s)", callee.Type())
	}
}

func (vm *VM) printTop() {
	topObj := vm.top()
	if topObj == nil {
		return
	}
	printFn := object.GetBuiltinFn("print")
	printFn.Fn(topObj)
	vm.lastPop = topObj
	vm.sp--
}

func (vm *VM) callFunc(fn object.CompiledFunc, numArgs int) error {
	if numArgs != fn.ParametersNum {
		return fmt.Errorf(format.Alert+"wrong number of arguments: want=%d, got=%d",
			fn.ParametersNum, numArgs)
	}
	newVars := make([]object.Object, fn.LocalsNum)
	// 块作用域的槽位不会在块退出时回收，读到"尚未赋值"的槽位应当得到 None，
	// 而不是 nil（那会在后续类型判断时 panic）
	for i := range newVars {
		newVars[i] = NullObj
	}
	for idx, arg := range vm.stack[vm.sp-numArgs : vm.sp] {
		newVars[idx] = arg
	}
	frame := NewFrame(fn.Instructions, &newVars, vm.sp-numArgs).WithLines(fn.Lines)
	frame.Name = fn.FnName
	frame.Line = fn.LineLoc
	return vm.pushFrame(frame)
}

// locate 给错误信息补上源码行号。
// 已经在消息里带行号的错误（如栈溢出）不再重复添加。
func (vm *VM) locate(err error) error {
	if err == nil || vm.frameIdx <= 0 {
		return err
	}
	msg := err.Error()
	if strings.Contains(msg, "line ") {
		return err
	}
	frame := vm.currentFrame()
	if frame == nil {
		return err
	}
	line := frame.LineAt(frame.ip)
	if line <= 0 {
		return err
	}
	return fmt.Errorf("%s%s: line %d: %s",
		format.Alert, frame.Name, line, strings.TrimPrefix(msg, format.Alert))
}

func (vm *VM) callBuiltin(builtin object.Builtin, argNums int) error {
	vm.argBuf = vm.argBuf[:0]
	vm.argBuf = append(vm.argBuf, vm.stack[vm.sp-argNums:vm.sp]...)
	result := builtin.Fn(vm.argBuf...)
	vm.sp = vm.sp - argNums - 1
	if vm.sp < 0 {
		vm.sp = 0
	}
	// 没有返回值的内建函数（如 print）在语言层面返回 None，
	// 这样"表达式一定在栈上留下一个值"的不变式才成立
	if result == nil {
		result = NullObj
	}
	return vm.push(result)
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
