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
	// InitLocalsSize 是所有帧共享的局部槽位区的初始容量（槽位数），按需翻倍
	InitLocalsSize = 256
)

var (
	StackIdxErr = fmt.Errorf(format.Alert + "invalid index")
	NullObj     = object.Null{}
)

// 解释执行时，整数运算结果和小布尔值是最高频的对象。
// 把它们做成单例，省掉每次装箱的一次堆分配（CPython 同样缓存小整数）。
const (
	smallIntMin = -256
	smallIntMax = 2048
)

var (
	smallInts [smallIntMax - smallIntMin + 1]object.Object
	trueObj   object.Object = object.Boolean{Value: true}
	falseObj  object.Object = object.Boolean{Value: false}
)

func init() {
	for i := range smallInts {
		smallInts[i] = object.Int{Value: i + smallIntMin}
	}
}

// intObj 返回整数对象，落在缓存区间内时复用单例。
func intObj(v int) object.Object {
	if v >= smallIntMin && v <= smallIntMax {
		return smallInts[v-smallIntMin]
	}
	return object.Int{Value: v}
}

func boolObj(b bool) object.Object {
	if b {
		return trueObj
	}
	return falseObj
}

type VM struct {
	constants []object.Object

	stack    []object.Object
	globals  []object.Object
	sp       int
	maxSP    int
	frames   []Frame
	frameIdx int
	// locals 是所有帧共享的局部槽位区：按栈方式分配/回收，
	// 这样每次函数调用都不必再 make 一个切片。
	// （CPython 3.11 用连续的 frame chunk 存放帧和槽位，思路相同。）
	locals    []object.Object
	localsTop int
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
		locals:   make([]object.Object, InitLocalsSize),
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

// LocalsDepth 返回共享局部槽位区当前被占用的槽位数。
// 帧返回后应当回到调用前的高度，用来验证"调用不再泄漏槽位"。
func (vm *VM) LocalsDepth() int {
	return vm.localsTop
}

func (vm *VM) push(obj object.Object) error {
	if vm.sp == len(vm.stack) {
		if err := vm.growStack(); err != nil {
			return err
		}
	}
	vm.put(obj)
	return nil
}

// growStack 把操作数栈翻倍；到上限时报栈溢出。
func (vm *VM) growStack() error {
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
	return nil
}

// put 直接写到栈顶。调用者必须保证栈上还有空间：
// Run 在每条指令前统一检查一次（一条指令最多压入一个值），
// 这样热路径上不必每条指令都做函数调用 + 边界检查。
func (vm *VM) put(obj object.Object) {
	vm.stack[vm.sp] = obj
	vm.sp++
	if vm.sp > vm.maxSP {
		vm.maxSP = vm.sp
	}
}

// readU16 读大端 2 字节操作数；直接用下标避免生成切片头。
func readU16(ins code.Instructions, pos int) uint16 {
	return uint16(ins[pos])<<8 | uint16(ins[pos+1])
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
	var op code.Opcode
	var varIdx uint16

	// main 帧拥有独立的局部槽位；全局变量单独放在 vm.globals 里
	// 同一个 VM 可以被重新 Run（REPL/重试），因此执行状态要复位：
	// 上一次如果在中途报错，帧栈和操作数栈都可能是有残留的。
	vm.sp = 0
	vm.maxSP = 0
	vm.frameIdx = 1
	vm.localsTop = 0
	mainVars := vm.allocLocals(bytecode.MainLocals)
	vm.frames[0] = NewFrame(bytecode.Instruction, mainVars, 0).WithLines(bytecode.Lines)
	vm.frames[0].localsBase = 0
	vm.frames[0].Name = "<main>"
	vm.constants = bytecode.Constants

	// 把"当前帧"和它的指令流、指令指针提到局部变量里。
	// 这是解释器循环里最值得做的一处优化：Frame 是个大结构体，
	// 每执行一条指令都要做几次 vm.currentFrame() 意味着
	// "取 frameIdx + 边界检查 + 乘以 sizeof(Frame) + 解引用"，
	// 而这三样在整个帧的生命周期内都不会变。
	// （CPython 3.11 的"帧留在寄存器/局部变量里"是同一个思路。）
	// 只有在 push/pop 帧之后才需要重新装载。
	frame := vm.currentFrame()
	ins := frame.ins
	ip := frame.ip

	// 统一的错误定位：任何从指令执行里冒出来的错误都补上源码行，
	// 这样调用方拿到的错误信息能直接指到源码（不需要逐处 return 时处理）
	defer func() {
		// ip 平时只存在于局部变量里，出错/退出时写回，供 locate 定位
		if n := vm.frameIdx; n > 0 && n <= len(vm.frames) {
			vm.frames[n-1].ip = ip
		}
		if err != nil {
			err = vm.locate(err)
		}
	}()

	for ip < len(ins)-1 {
		ip++
		op = code.Opcode(ins[ip])
		// 一条指令最多压入一个值：在这里统一保证操作数栈还有空位，
		// 热路径上的 put 就不用再做检查了。
		if vm.sp >= len(vm.stack) {
			if err := vm.growStack(); err != nil {
				return err
			}
		}
		switch op {
		case code.OpConstant:
			constIdx := readU16(ins, ip+1)
			ip += 2
			vm.put(vm.constants[constIdx])
		case code.OpTrue, code.OpFalse:
			if op == code.OpTrue {
				vm.put(nativeBoolToBool(true))
			} else {
				vm.put(nativeBoolToBool(false))
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
			vm.put(NullObj)
		case code.OpJump:
			pos := int(readU16(ins, ip+1))
			ip = pos - 1
		case code.OpJumpNotTrue:
			pos := int(readU16(ins, ip+1))
			ip += 2 //skip the operand of code.OpJumpNotTrue
			cond := vm.pop()
			boolVal := objToNativeBool(cond)
			if !boolVal {
				ip = pos - 1
			}
		case code.OpSetGlobal:
			varIdx = readU16(ins, ip+1)
			ip += 2 //skip the operand of code.OpSetGlobal
			if err := vm.setGlobal(varIdx, vm.pop()); err != nil {
				return err
			}
		case code.OpGetGlobal:
			varIdx = readU16(ins, ip+1)
			ip += 2 //skip the operand of code.OpGetGlobal
			vm.put(vm.globalAt(varIdx))
		case code.OpUpdateGlobal:
			varIdx = readU16(ins, ip+1)
			ip += 2 //skip the operand of code.OpUpdate
			if err := vm.setGlobal(varIdx, vm.top()); err != nil {
				return err
			}
		case code.OpCallFunc:
			numArgs := ins[ip+1]
			ip++
			// 调用可能压入新帧：先把当前位置写回，返回时才能接着执行
			frame.ip = ip
			if err := vm.executeCall(int(numArgs)); err != nil {
				return err
			}
			// 帧数组可能扩容（指针失效），必须重新装载
			frame = vm.currentFrame()
			ins, ip = frame.ins, frame.ip
		case code.OpReturnVal:
			returnVal := vm.pop()
			if vm.frameIdx <= 1 {
				return fmt.Errorf(format.Alert + "return outside of a function")
			}

			ret := vm.popFrame()
			vm.sp = ret.basePoint - 1
			if vm.sp < 0 {
				vm.sp = 0
			}
			// 回收这个帧的局部槽位：整段直接还给槽位区，无需逐个清理
			if ret.localsBase >= 0 && ret.localsBase <= vm.localsTop {
				vm.localsTop = ret.localsBase
			}

			vm.put(returnVal)
			frame = vm.currentFrame()
			ins, ip = frame.ins, frame.ip
		case code.OpSetLocal:
			varIdx = readU16(ins, ip+1)
			ip += 2
			frame.vars[varIdx] = vm.pop()
		case code.OpGetLocal:
			varIdx = readU16(ins, ip+1)
			ip += 2
			// 热路径，内联 localAt：越界/未赋值都返回 None
			vars := frame.vars
			if int(varIdx) < len(vars) && vars[varIdx] != nil {
				vm.put(vars[varIdx])
			} else {
				vm.put(NullObj)
			}
		case code.OpUpdateLocal:
			varIdx = readU16(ins, ip+1)
			ip += 2
			frame.vars[varIdx] = vm.top()
		case code.OpGetBuiltin:
			builtinIdx := ins[ip+1]
			ip += 1
			vm.put(object.BuiltinFns[builtinIdx].Builtin)
		case code.OpClosure:
			fnIdx := ins[ip+1]
			ip += 1
			vm.put(vm.constants[fnIdx])
		case code.OpBuildArray:
			gap := int(readU16(ins, ip+1))
			ip += 2
			ele := make([]object.Object, gap)
			copy(ele, vm.stack[vm.sp-gap:vm.sp])
			vm.sp -= gap //栈指针归位
			vm.put(object.Array{Elements: ele})
		case code.OpMakeSlice:
			sliceObj := object.Slice{}
			sliceObj.Start = vm.stack[vm.sp-3]
			sliceObj.End = vm.stack[vm.sp-2]
			sliceObj.Step = vm.stack[vm.sp-1]
			vm.sp -= 3
			vm.put(sliceObj)
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
			keyLen := readU16(ins, ip+1)
			ip += 2
			err := vm.makeMap(int(keyLen))
			if err != nil {
				return err
			}
		case code.OpLoadMethod:
			varIdx = readU16(ins, ip+1)
			ip += 2
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
			argsNum := ins[ip+1]
			ip += 1
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
	// 用类型断言代替 Type() 字符串比较：Type() 返回的是字符串，
	// 每做一次算术都要比较好几轮字符串，而接口断言只比 itab 指针。
	if l, ok := left.(object.Int); ok {
		if r, ok := right.(object.Int); ok {
			return vm.executeBinOpInt(op, l.Value, r.Value)
		}
	}
	if _, ok := left.(object.String); ok {
		return vm.executeBinOpStr(op, left, right)
	}
	if _, ok := right.(object.String); ok {
		return vm.executeBinOpStr(op, left, right)
	}
	if _, ok := right.(object.Float); ok {
		return vm.executeBinOpFloat(op, left, right)
	}
	if _, ok := left.(object.Float); ok {
		return vm.executeBinOpFloat(op, left, right)
	}
	return fmt.Errorf(format.Alert+"unsupported types for binary operation: %s(%s) %s(%s) %s",
		left.Type(), left.Inspect(), right.Type(), right.Inspect(), code.Definitions[op].Name)
}

func (vm *VM) compareBinOp(op code.Opcode) error {
	right := vm.pop()
	left := vm.top()
	if lv, ok := numOf(left); ok {
		if rv, ok := numOf(right); ok {
			var res bool
			switch op {
			case code.OpEqual:
				res = lv == rv
			case code.OpNotEQ:
				res = lv != rv
			case code.OpGT:
				res = lv > rv
			case code.OpGTEq:
				res = lv >= rv
			case code.OpLT:
				res = lv < rv
			case code.OpLTEq:
				res = lv <= rv
			default:
				return fmt.Errorf(format.Alert+"unsupport operator for NumberObj: %d", op)
			}
			return vm.replace(boolObj(res))
		}
	}
	if _, ok := left.(object.String); ok {
		if _, ok := right.(object.String); ok {
			return vm.compareStringObj(op, left, right)
		}
	}
	return vm.compareObj(op, left, right)
}

// numOf 把数值对象取成 float64；比较一律按浮点语义（与原实现一致）。
func numOf(obj object.Object) (float64, bool) {
	switch v := obj.(type) {
	case object.Int:
		return float64(v.Value), true
	case object.Float:
		return v.Value, true
	}
	return 0, false
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

// compareNumObj 已合并进 compareBinOp：数值比较不再需要为每个对象做类型分支。
func nativeBoolToBool(b bool) object.Object {
	return boolObj(b)
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

func (vm *VM) executeBinOpInt(op code.Opcode, leftVal, rightVal int) error {
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
	return vm.replace(intObj(res))
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

// allocLocals 从共享槽位区里切出 n 个槽位给新帧，并清零成 None。
// 槽位区按栈方式使用：帧返回时把 localsTop 退回去即可，不需要回收动作。
// 扩容时已经存在的帧要把 vars 重新指向新数组。
func (vm *VM) allocLocals(n int) []object.Object {
	if n <= 0 {
		return nil
	}
	end := vm.localsTop + n
	if end > len(vm.locals) {
		grown := len(vm.locals) * 2
		if grown < end {
			grown = end
		}
		grownLocals := make([]object.Object, grown)
		copy(grownLocals, vm.locals[:vm.localsTop])
		vm.locals = grownLocals
		for i := 0; i < vm.frameIdx && i < len(vm.frames); i++ {
			f := &vm.frames[i]
			if f.localsBase+len(f.vars) <= len(grownLocals) {
				f.vars = grownLocals[f.localsBase : f.localsBase+len(f.vars)]
			}
		}
	}
	vars := vm.locals[vm.localsTop:end]
	for i := range vars {
		vars[i] = NullObj
	}
	vm.localsTop = end
	return vars
}

// pushFrame 在帧栈顶原地构造一个新帧并返回它的指针。
// 这样调用路径上不再出现 Frame 值拷贝（Frame 有一百多字节）。
func (vm *VM) pushFrame(calleeName string, calleeLine int) (*Frame, error) {
	if vm.frameIdx >= MaxFrame {
		return nil, fmt.Errorf("%scall stack overflow: call depth exceeded %d while calling %s (line %d)",
			format.Alert, MaxFrame, calleeName, calleeLine)
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
	f := &vm.frames[vm.frameIdx]
	// 只重置 ip：其余字段由调用方（callFunc）逐个赋值。
	// 注意：Frame 新增字段时必须同时在这里或调用方初始化，否则会读到上一层的残留值。
	f.ip = -1
	vm.frameIdx++
	return f, nil
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
		// 传指针：CompiledFunc 有一百多字节，按值再传一次就多一次拷贝
		return vm.callFunc(&callee, numArgs)
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

func (vm *VM) callFunc(fn *object.CompiledFunc, numArgs int) error {
	if numArgs != fn.ParametersNum {
		return fmt.Errorf(format.Alert+"wrong number of arguments: want=%d, got=%d",
			fn.ParametersNum, numArgs)
	}
	// 块作用域的槽位不会在块退出时回收，读到"尚未赋值"的槽位应当得到 None，
	// 而不是 nil（那会在后续类型判断时 panic）——allocLocals 已经清零。
	base := vm.localsTop
	newVars := vm.allocLocals(fn.LocalsNum)
	args := vm.stack[vm.sp-numArgs : vm.sp]
	for idx, arg := range args {
		newVars[idx] = arg
	}
	// 帧直接在其槽位里原地构造，避免 Frame 这个上百字节的结构体被拷来拷去
	frame, err := vm.pushFrame(fn.FnName, fn.LineLoc)
	if err != nil {
		vm.localsTop = base // 帧没建起来，把刚切出去的槽位还回去
		return err
	}
	frame.ins = fn.Instructions
	frame.lines = fn.Lines
	frame.vars = newVars
	frame.basePoint = vm.sp - numArgs
	frame.localsBase = base
	frame.Name = fn.FnName
	frame.Line = fn.LineLoc
	return nil
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
	cur := vm.currentFrame()
	fmt.Println("Current Frame:" + strconv.Itoa(vm.frameIdx) + " Instruction Pointer:" +
		strconv.Itoa(cur.ip-operandWidth))
	fmt.Println("Stack: "+sb.String()+"  Current Pointer:", vm.sp)
}
