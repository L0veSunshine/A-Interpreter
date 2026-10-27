package object

import (
	"Interpreter/format"
	"fmt"
)

func FindMethod(objType ObjType, methodName string) (Object, error) {
	var list ObjMethods
	switch objType {
	case ArrayObj:
		list = ArrayMethodList
	case StringObj:
		list = StringMethodList
	default:
		return nil, fmt.Errorf(format.Alert+"Object %s don't has method %s", string(objType), methodName)
	}
	method, ok := list[methodName]
	if !ok || method.M == nil {
		return nil, fmt.Errorf(format.Alert+"Object %s don't has method %s", string(objType), methodName)
	}
	return method, nil
}
