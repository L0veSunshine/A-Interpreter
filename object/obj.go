package object

import (
	"Interpreter/code"
	"strconv"
	"unsafe"
)

type BaseObject struct {
	Type string
}

type IntObject struct {
	Type string
	IVal int
}

type FloatObject struct {
	Type string
	FVal float64
}

type BooleanObject struct {
	Type string
	BVal bool
}

type StringObject struct {
	Type string
	SVal string
}

type NullObject struct {
	Type string
}

type CFunc struct {
	Type          string
	FnName        string
	Instructions  code.Instructions
	LocalsNum     int
	ParametersNum int
	Called        bool
}

type BFunc struct {
	Type string
	Fn   BuiltinFunction
}

func Inspect(obj *BaseObject) string {
	switch obj.Type {
	case IntObj:
		rev := (*IntObject)(unsafe.Pointer(obj))
		return strconv.Itoa(rev.IVal)
	case FloatObj:
		var s string
		rev := (*FloatObject)(unsafe.Pointer(obj))
		if rev.FVal < 10e12 {
			s = strconv.FormatFloat(rev.FVal, 'f', -1, 64)
		} else {
			s = strconv.FormatFloat(rev.FVal, 'e', 12, 64)
		}
		return s
	case BooleanObj:
		rev := (*BooleanObject)(unsafe.Pointer(obj))
		return strconv.FormatBool(rev.BVal)
	case StringObj:
		rev := (*StringObject)(unsafe.Pointer(obj))
		return rev.SVal
	case CompiledFuncObj:
		rev := (*CFunc)(unsafe.Pointer(obj))
		return "Func" + rev.FnName
	case BuiltinObj:
		return "BuiltIn Func"
	default:
		return ""
	}
}
