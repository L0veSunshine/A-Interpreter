package vm

import (
	"Interpreter/code"
	"Interpreter/object"
)

type Frame struct {
	fn *object.CompiledFunc
	ip,
	basePoint int
}

func NewFrame(fn *object.CompiledFunc, basePoint int) *Frame {
	return &Frame{
		fn:        fn,
		ip:        -1,
		basePoint: basePoint,
	}
}

func (f Frame) Instructions() code.Instructions {
	return f.fn.Instructions
}
