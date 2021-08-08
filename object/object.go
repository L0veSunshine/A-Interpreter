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

type String struct {
	Value []rune
}

func (s String) Type() ObjType {
	return StringObj
}

func (s String) Inspect() string {
	return string(s.Value)
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
	FnName        string
	Instructions  code.Instructions
	LocalsNum     int
	ParametersNum int
	Called        bool
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
