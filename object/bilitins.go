package object

import (
	"fmt"
	"strconv"
	"strings"
)

type BuiltinFn struct {
	Name    string
	Builtin Builtin
}

var BuiltinFns = []BuiltinFn{
	{
		"print",
		Builtin{Fn: func(args ...Object) Object {
			var sb strings.Builder
			if len(args) > 0 {
				for idx, arg := range args {
					if idx == len(args)-1 {
						sb.WriteString(arg.Inspect())
					} else {
						sb.WriteString(arg.Inspect() + " ")
					}
				}
				fmt.Println(sb.String())
			}
			return nil
		}},
	},
	{
		"len",
		Builtin{Fn: func(args ...Object) Object {
			if obj := checkArgs("len", args, 1); obj != nil {
				return obj
			}
			arg := args[0]
			return builtLen(arg)
		}},
	},
	{
		"type",
		Builtin{Fn: func(args ...Object) Object {
			if obj := checkArgs("type", args, 1); obj != nil {
				return obj
			}
			arg := args[0]
			return String{Value: []rune(fmt.Sprintf("<class '%s'>", arg.Type()))}
		}},
	},
	{
		"int",
		Builtin{Fn: func(args ...Object) Object {
			if obj := checkArgs("int", args, 1); obj != nil {
				return obj
			}
			arg := args[0]
			return builtInt(arg)
		}},
	},
	{
		"float",
		Builtin{Fn: func(args ...Object) Object {
			if obj := checkArgs("float", args, 1); obj != nil {
				return obj
			}
			arg := args[0]
			return builtFloat(arg)
		}},
	},
}

func GetBuiltinFn(name string) Builtin {
	for _, f := range BuiltinFns {
		if f.Name == name {
			return f.Builtin
		}
	}
	return Builtin{}
}

func checkArgs(funcName string, args []Object, ArgsNum int) Object {
	if len(args) != ArgsNum {
		return Error{ErrorMsg: fmt.Sprintf("%s() takes exactly one argument (%d given)",
			funcName, len(args))}
	}
	return nil
}

func builtLen(arg Object) Object {
	var length int
	switch arg := arg.(type) {
	case String:
		length = len(arg.Value)
	case Array:
		length = len(arg.Elements)
	case Map:
		length = arg.Size
	default:
		return Error{ErrorMsg: fmt.Sprintf("len don't support type %s.", arg.Type())}
	}
	return Int{Value: length}
}

func builtInt(arg Object) Object {
	var res int
	switch arg := arg.(type) {
	case Int:
		return arg
	case Float:
		res = int(arg.Value)
	case String:
		var err error
		res, err = strconv.Atoi(string(arg.Value))
		if err != nil {
			return Error{ErrorMsg: err.Error()}
		}
	case Boolean:
		b := arg.Value
		if b {
			res = 1
		} else {
			res = 0
		}
	default:
		return Error{ErrorMsg: fmt.Sprintf("int() don't support type %s.", arg.Type())}
	}
	return Int{Value: res}
}

func builtFloat(arg Object) Object {
	var res float64
	switch arg := arg.(type) {
	case Int:
		res = float64(arg.Value)
	case Float:
		return arg
	case String:
		var err error
		res, err = strconv.ParseFloat(string(arg.Value), 64)
		if err != nil {
			return Error{ErrorMsg: err.Error()}
		}
	default:
		return Error{ErrorMsg: fmt.Sprintf("float() don't support type %s.", arg.Type())}
	}
	return Float{Value: res}
}
