package object

import (
	"Interpreter/code"
	"Interpreter/format"
	"fmt"
	"strings"
)

// ObjGOFunc 是内建方法的实现。
//
// 返回 (新的 self, 返回值, 错误)：
//   - self 可能被整体替换（容器扩容后 slice 头变了），调用方必须把新的 self
//     写回变量——这正是 `a.append(x)` 能改到 `a` 的原因；
//   - 错误必须让调用方感知。早期版本用 `[]Object` 作返回值，错误只能塞进
//     "self"那一格，结果 `s.split(1)` 这种类型错误会被静默吞掉并把 Error
//     对象写进变量。
type ObjGOFunc func(self Object, args []Object) (Object, Object, error)

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
	// MaxStack 是执行本函数需要的操作数栈上界（相对帧基址的形参之后）。
	// 由编译期数据流算出（见 code.MaxStack），VM 建帧时一次性预留；
	// 0 表示未知，此时 VM 退回保守预留。
	MaxStack int
	Called   bool
	LineLoc  int
	// ScopeID 对应解析期的函数作用域，反汇编时用它定位正确的符号表
	ScopeID  int
	FreeVars []FreeVar
}

// FreeVar selects a shared cell from the defining frame or its closure.
type FreeVar struct {
	Local bool
	Index int
}

type Cell struct {
	Value Object
}

type Closure struct {
	Fn   CompiledFunc
	Free []*Cell
}

func (c *Closure) Type() ObjType {
	return CompiledFuncObj
}

func (c *Closure) Inspect() string {
	return fmt.Sprintf("Closure[%s:%p]", c.Fn.FnName, c)
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

// RichCompare 以"同类型且 Inspect 相同"判定相等，是 Equal 的语义基准。
// 它会为两边各分配一次字符串，热路径应使用 Equal。
func RichCompare(v, w Object) bool {
	res := v.Type() == w.Type()
	res = res && (v.Inspect() == w.Inspect())
	return res
}
