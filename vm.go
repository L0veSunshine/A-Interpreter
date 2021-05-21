package main

import (
	"Interpreter/code"
	"Interpreter/object"
	"errors"
	"fmt"
	"math"
)

const StackSize = 2048

type VM struct {
	constants    []object.Object
	instructions code.Instructions
	stack        []object.Object
	sp           int
}

func NewVM() *VM {
	return &VM{
		constants:    nil,
		instructions: nil,
		stack:        make([]object.Object, StackSize),
		sp:           0,
	}
}

func (vm *VM) LastPop() object.Object {
	return vm.stack[vm.sp]
}

func (vm *VM) push(obj object.Object) error {
	if vm.sp+1 > StackSize {
		return fmt.Errorf("stack overflow")
	}
	vm.stack[vm.sp] = obj
	vm.sp++
	return nil
}

func (vm *VM) pop() object.Object {
	obj := vm.stack[vm.sp-1]
	vm.sp--
	return obj
}

func (vm *VM) Run(bytecode *Bytecode) error {
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
		case code.OpPop:
			vm.pop()
		case code.OpAdd, code.OpSub, code.OpMul, code.OpDiv, code.OpPow, code.OpFloor:
			err := vm.executeBinOp(op)
			if err != nil {
				return err
			}
		case code.OpPlus, code.OpMinus:
			err := vm.executePrefix(op)
			if err != nil {
				return err
			}
		case code.OpEqual, code.OpNotEQ, code.OpGTEq, code.OpGT:
			err := vm.compareBinOp(op)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (vm *VM) executeBinOp(op code.Opcode) error {
	right := vm.pop()
	left := vm.pop()
	switch {
	case right.Type() == object.NumberObj && left.Type() == object.NumberObj:
		return vm.executeBinOpNum(op, left, right)
	case right.Type() == object.StringObj && left.Type() == object.StringObj:
		return vm.executeBinOpStr(op, left, right)
	}

	return fmt.Errorf("unsupported types for binary operation: %s %s",
		left.Type(), right.Type())
}

func (vm *VM) compareBinOp(op code.Opcode) error {
	right := vm.pop()
	left := vm.pop()
	if left.Type() == object.NumberObj && right.Type() == object.NumberObj {
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
	return vm.push(nativeBoolToBool(res))
}

func (vm *VM) compareNumObj(op code.Opcode, left, right object.Object) error {
	leftVal := left.(object.Number).Value
	rightVal := right.(object.Number).Value
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
	return vm.push(nativeBoolToBool(res))
}

func nativeBoolToBool(b bool) object.Boolean {
	return object.Boolean{Value: b}
}

func (vm *VM) executeBinOpNum(op code.Opcode, left, right object.Object) error {
	leftVal := left.(object.Number).Value
	rightVal := right.(object.Number).Value
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
	case code.OpFloor:
		res = math.Floor(leftVal / rightVal)
	case code.OpPow:
		res = math.Pow(leftVal, rightVal)
	default:
		return fmt.Errorf("unknown integer operator: %d", op)
	}
	return vm.push(object.Number{Value: res})
}

func (vm *VM) executeBinOpStr(op code.Opcode, left, right object.Object) error {
	leftVal := left.(object.String).Value
	rightVal := right.(object.String).Value
	if op == code.OpAdd {
		return vm.push(object.String{Value: leftVal + rightVal})
	} else {
		return fmt.Errorf("unknown integer operator: %d", op)
	}
}

func (vm *VM) executePrefix(op code.Opcode) error {
	operand := vm.pop()
	if operand.Type() != object.NumberObj {
		return fmt.Errorf("unsupported type for negation: %s", operand.Type())
	}
	value := operand.(object.Number).Value
	switch op {
	case code.OpMinus:
		value = -value
	case code.OpPlus:
	default:
		return fmt.Errorf("unkonwn opCode %s", string(op))
	}
	return vm.push(object.Number{Value: value})
}
