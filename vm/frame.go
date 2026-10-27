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
	// lines 是"指令偏移 -> 源码行"映射，报错时用来定位源码行
	lines []code.LineEntry
	// Name/Line 用于错误信息定位（栈溢出时报出是哪个函数）
	Name string
	Line int
}

func NewFrame(ins code.Instructions, vars *[]object.Object, basePoint int) Frame {
	return Frame{
		ins:       ins,
		ip:        -1,
		vars:      *vars,
		basePoint: basePoint,
	}
}

// WithLines 附带"指令偏移 -> 源码行"映射，用于报错定位。
func (f Frame) WithLines(lines []code.LineEntry) Frame {
	f.lines = lines
	return f
}

// LineAt 返回当前指令对应的源码行（0 表示未知）。
func (f *Frame) LineAt(ip int) int {
	return code.LineAt(f.lines, ip)
}

func (f *Frame) Instructions() code.Instructions {
	return f.ins
}
