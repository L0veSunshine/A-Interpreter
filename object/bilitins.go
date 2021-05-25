package object

import "fmt"

type BuiltinFn struct {
	Name    string
	Builtin *Builtin
}

var BuiltinFns = []BuiltinFn{
	{
		"print",
		&Builtin{Fn: func(args ...Object) Object {
			for _, arg := range args {
				fmt.Println(arg.Inspect())
			}
			return nil
		},
		},
	},
}

func GetBuiltinFn(name string) *Builtin {
	for _, f := range BuiltinFns {
		if f.Name == name {
			return f.Builtin
		}
	}
	return nil
}
