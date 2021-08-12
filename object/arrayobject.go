package object

import "strings"

type Array struct {
	Elements []Object
}

func (a Array) Type() ObjType {
	return ArrayObj
}
func InspectArrayObj(object Object) string {
	switch object.Type() {
	case StringObj:
		return `'` + (object.Inspect()) + `'`
	default:
		return object.Inspect()
	}
}

func (a Array) Inspect() string {
	var sb strings.Builder
	if len(a.Elements) > 0 {
		sb.WriteString("[")
		for i := 0; i < len(a.Elements)-1; i++ {
			sb.WriteString(InspectArrayObj(a.Elements[i]) + ", ")
		}
		sb.WriteString(InspectArrayObj(a.Elements[len(a.Elements)-1]) + "]")
		return sb.String()
	}
	return "[]"
}

func arrayAppend(self Object, args ...Object) []Object {
	obj := self.(Array)
	arr := &obj
	arr.Elements = append(arr.Elements, args...)
	return []Object{*arr, nullObj}
}

func arrayPop(self Object, _ ...Object) []Object {
	obj := self.(Array)
	arr := &obj
	idx := len(arr.Elements) - 1
	if idx < 0 {
		return []Object{*arr, Error{ErrorMsg: "Index out of range."}}
	}
	tar := arr.Elements[idx]
	arr.Elements = arr.Elements[:idx]
	return []Object{*arr, tar}
}

func arrayReverse(self Object, _ ...Object) []Object {
	obj := self.(Array)
	arr := &obj
	for i, j := 0, len(arr.Elements)-1; i < j; {
		arr.Elements[i], arr.Elements[j] = arr.Elements[j], arr.Elements[i]
		i++
		j--
	}
	return []Object{*arr, nullObj}
}

var ArrayMethodList = ObjMethods{
	"append":  MethodObj{arrayAppend},
	"pop":     MethodObj{arrayPop},
	"reverse": MethodObj{arrayReverse},
}
