package object

import "strings"

type Array struct {
	Elements []Object
}

func (a Array) Type() ObjType {
	return ArrayObj
}

func (a Array) Inspect() string {
	var sb strings.Builder
	if len(a.Elements) > 0 {
		sb.WriteString("[")
		for i := 0; i < len(a.Elements)-1; i++ {
			sb.WriteString(a.Elements[i].Inspect() + ", ")
		}
		sb.WriteString(a.Elements[len(a.Elements)-1].Inspect() + "]")
		return sb.String()
	}
	return "[]"
}

func arrayAppend(self Object, args ...Object) []Object {
	obj := self.(Array)
	arr := &obj
	arr.Elements = append(arr.Elements, args...)
	return []Object{*arr, Null{}}
}

func arrayPop(self Object, args ...Object) []Object {
	obj := self.(Array)
	arr := &obj
	idx := len(arr.Elements) - 1
	tar := arr.Elements[idx]
	arr.Elements = arr.Elements[:idx]
	return []Object{*arr, tar}
}

var ArrayMethodList = ObjMethods{
	"append": MethodObj{arrayAppend},
	"pop":    MethodObj{arrayPop},
}
