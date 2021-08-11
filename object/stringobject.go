package object

import (
	"fmt"
	"strings"
)

type String struct {
	Value []rune
}

func (s String) Type() ObjType {
	return StringObj
}

func (s String) Inspect() string {
	return string(s.Value)
}
func stringSplit(self Object, args ...Object) []Object {
	rs := self.(String).Value
	if len(args) != 1 {
		return []Object{Error{ErrorMsg: fmt.Sprintf("want 1 arg but get %d", len(args))}}
	}
	arg := args[0]
	if arg.Type() != StringObj {
		return []Object{Error{ErrorMsg: fmt.Sprintf("TypeError: must be str or None, not %s", arg.Type())}}
	}
	target := arg.(String).Value
	subStr := strings.Split(string(rs), string(target))
	var res []Object
	for _, s := range subStr {
		res = append(res, String{Value: []rune(s)})
	}
	return []Object{self, Array{Elements: res}}
}

var StringMethodList = ObjMethods{
	"split": MethodObj{M: stringSplit},
}
