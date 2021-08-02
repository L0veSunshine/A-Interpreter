package object

import (
	"fmt"
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
		},
		},
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
