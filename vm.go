package main

import (
	"Interpreter/code"
	"Interpreter/object"
	"errors"
	"fmt"
	"math"
)

const (
	StackSize  = 2048
	GlobalSize = 65536
)

var (
	StackOverErr = fmt.Errorf("stack overflow")
	StackIdxErr  = fmt.Errorf("invalid index")
	NullObj      = object.Null{}
)

type VM struct {
	constants    []object.Object
	instructions code.Instructions
	stack        [StackSize]object.Object
	globals      [GlobalSize]object.Object
	sp           int
}

func NewVM() *VM {
	return &VM{
		constants:    nil,
		instructions: nil,
		sp:           0,
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

func (vm *VM) Run(bytecode *code.Bytecode) error {
	vm.instructions = bytecode.Instruction
	vm.constants = bytecode.Constants
	for ip := 0; ip < len(vm.instructions); ip++ {
		op := code.Opcode(vm.instructions[ip])
		switch op {
		case code.OpConstant:
			constIdx := code.ReadUint16(vm.instructions[ip+1:])
			ip += 2
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
			pos := int(code.ReadUint16(vm.instructions[ip+1:]))
			ip = pos - 1
		case code.OpJumpNotTrue:
			pos := int(code.ReadUint16(vm.instructions[ip+1:]))
			ip += 2 //skip the operand of code.OpJumpNotTrue
			cond := vm.pop()
			boolVal := objToNativeBool(cond)
			if !boolVal {
				ip = pos - 1
			}
		case code.OpSetGlobal:
			globalIdx := code.ReadUint16(vm.instructions[ip+1:])
			ip += 2 //skip the operand of code.OpSetGlobal
			vm.globals[globalIdx] = vm.pop()
		case code.OpGetGlobal:
			globalIdx := code.ReadUint16(vm.instructions[ip+1:])
			ip += 2 //skip the operand of code.OpGetGlobal
			err := vm.push(vm.globals[globalIdx])
			if err != nil {
				return err
			}
		case code.OpUpdate:
			globalIdx := code.ReadUint16(vm.instructions[ip+1:])
			ip += 2 //skip the operand of code.OpUpdate
			vm.globals[globalIdx] = vm.top()
			if vm.sp > 1 {
				vm.sp--
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
	case right.Type() == object.FloatObj || left.Type() == object.FloatObj:
		return vm.executeBinOpFloat(op, left, right)
	case right.Type() == object.StringObj && left.Type() == object.StringObj:
		return vm.executeBinOpStr(op, left, right)
	}
	return fmt.Errorf("unsupported types for binary operation: %s %s",
		left.Type(), right.Type())
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
		return fmt.Errorf("unknown integer operator: %d", op)
	}
	return vm.replace(object.Float{Value: res})
}

func (vm *VM) executeBinOpStr(op code.Opcode, left, right object.Object) error {
	leftVal := left.(object.String).Value
	rightVal := right.(object.String).Value
	if op == code.OpAdd {
		return vm.replace(object.String{Value: leftVal + rightVal})
	} else {
		return fmt.Errorf("unknown integer operator: %d", op)
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
