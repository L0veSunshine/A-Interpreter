package vm

import (
	"Interpreter/code"
	"Interpreter/object"
)

type Frame struct {
	ins code.Instructions
	ip,
	basePoint int
	vars []object.Object
}

func NewFrame(ins code.Instructions, vars []object.Object, basePoint int) Frame {
	return Frame{
		ins:       ins,
		ip:        -1,
		vars:      vars,
		basePoint: basePoint,
	}
}

func (f *Frame) Instructions() code.Instructions {
	return f.ins
}
