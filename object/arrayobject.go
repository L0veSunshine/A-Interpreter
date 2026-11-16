package object

import (
	"fmt"
	"strings"

	"Interpreter/format"
)

// Array 是引用语义的可变数组：赋值、传参共享同一个 *Array，
// append/pop/reverse 原地修改。指针装入接口不需要分配，
// 因此 append 不再为每次调用重新装箱一个 Array 结构体。
type Array struct {
	Elements []Object
}

func (a *Array) Type() ObjType {
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

func (a *Array) Inspect() string {
	if len(a.Elements) == 0 {
		return "[]"
	}
	var sb strings.Builder
	sb.WriteString("[")
	for i, el := range a.Elements {
		if i > 0 {
			sb.WriteString(", ")
		}
		if el.Type() == StringObj {
			sb.WriteString("'")
			sb.WriteString(el.Inspect())
			sb.WriteString("'")
		} else {
			sb.WriteString(el.Inspect())
		}
	}
	sb.WriteString("]")
	return sb.String()
}

func arrayAppend(self Object, args []Object) (Object, Object, error) {
	arr, ok := self.(*Array)
	if !ok {
		return self, nil, fmt.Errorf(format.Alert+"append() needs an Array, got %s", self.Type())
	}
	arr.Elements = append(arr.Elements, args...)
	return arr, nullObj, nil
}

func arrayPop(self Object, _ []Object) (Object, Object, error) {
	arr, ok := self.(*Array)
	if !ok {
		return self, nil, fmt.Errorf(format.Alert+"pop() needs an Array, got %s", self.Type())
	}
	idx := len(arr.Elements) - 1
	if idx < 0 {
		return arr, nil, fmt.Errorf(format.Alert + "Index out of range.")
	}
	tar := arr.Elements[idx]
	arr.Elements = arr.Elements[:idx]
	return arr, tar, nil
}

func arrayReverse(self Object, _ []Object) (Object, Object, error) {
	arr, ok := self.(*Array)
	if !ok {
		return self, nil, fmt.Errorf(format.Alert+"reverse() needs an Array, got %s", self.Type())
	}
	for i, j := 0, len(arr.Elements)-1; i < j; {
		arr.Elements[i], arr.Elements[j] = arr.Elements[j], arr.Elements[i]
		i++
		j--
	}
	return arr, nullObj, nil
}

func arrayIndex(self Object, target []Object) (Object, Object, error) {
	if len(target) == 0 {
		return self, nil, fmt.Errorf(format.Alert + "no args")
	}
	tar := target[0]
	arr, ok := self.(*Array)
	if !ok {
		return self, nil, fmt.Errorf(format.Alert+"index() needs an Array, got %s", self.Type())
	}
	var idx = -1
	for i, o := range arr.Elements {
		if ValueEqual(tar, o) {
			idx = i
			break
		}
	}
	return arr, Int{Value: idx}, nil
}

var ArrayMethodList = ObjMethods{
	"append":  MethodObj{arrayAppend},
	"pop":     MethodObj{arrayPop},
	"reverse": MethodObj{arrayReverse},
	"index":   MethodObj{M: arrayIndex},
}
