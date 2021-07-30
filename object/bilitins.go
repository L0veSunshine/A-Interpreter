package object

import "fmt"

type BuiltinFn struct {
	Name    string
	Builtin *BFunc
}

var BuiltinFns = []BuiltinFn{
	{
		"print",
		&BFunc{
			Type: BuiltinObj,
			Fn: func(args ...*BaseObject) *BaseObject {
				for _, arg := range args {
					fmt.Println(Inspect(arg))
				}
				return nil
			},
		},
	},
}

func GetBuiltinFn(name string) *BFunc {
	for _, f := range BuiltinFns {
		if f.Name == name {
			return f.Builtin
		}
	}
	return nil
}
