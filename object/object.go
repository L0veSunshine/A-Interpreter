package object

import (
	"Interpreter/code"
	"Interpreter/format"
	"fmt"
	"strings"
)

type ObjGOFunc func(object Object, args ...Object) []Object

type MethodObj struct {
	M ObjGOFunc
}

func (m MethodObj) Type() ObjType {
	return Method
}

func (m MethodObj) Inspect() string {
	return "Method for Object"
}

type ObjMethods map[string]MethodObj

type BuiltinFunction func(args ...Object) Object

type ObjType string

type Object interface {
	Type() ObjType
	Inspect() string
}

type Builtin struct {
	Fn BuiltinFunction
}

func (b Builtin) Type() ObjType {
	return BuiltinObj
}

func (b Builtin) Inspect() string {
	return "builtin function"
}

type CompiledFunc struct {
	FnName       string
	Instructions code.Instructions
	// Lines 是"指令偏移 -> 源码行"映射，用于把运行时错误定位到源码行
	Lines         []code.LineEntry
	LocalsNum     int
	ParametersNum int
	Called        bool
	LineLoc       int
	// ScopeID 对应解析期的函数作用域，反汇编时用它定位正确的符号表
	ScopeID int
}

func (cf CompiledFunc) Type() ObjType {
	return CompiledFuncObj
}

func (cf CompiledFunc) Inspect() string {
	return fmt.Sprintf("CompiledFunc[%p]", &cf)
}

type Slice struct {
	Start, End, Step Object
}

func (s Slice) Type() ObjType {
	return SliceObj
}

func (s Slice) Inspect() string {
	var sb strings.Builder
	sb.WriteString("[")
	sb.WriteString(s.Start.Inspect() + ":" + s.End.Inspect() + ":" + s.Step.Inspect())
	sb.WriteString("]")
	return sb.String()
}

type Error struct {
	ErrorMsg string
}

func (e Error) Type() ObjType {
	return ErrorObj
}

func (e Error) Inspect() string {
	return format.Error + "Error: " + e.ErrorMsg
}

func RichCompare(v, w Object) bool {
	res := v.Type() == w.Type()
	res = res && (v.Inspect() == w.Inspect())
	return res
}
