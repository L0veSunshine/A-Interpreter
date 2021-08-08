package object

import (
	"Interpreter/format"
	"fmt"
)

func FindMethod(objType ObjType, methodName string) (Object, error) {
	switch objType {
	case ArrayObj:
		return ArrayMethodList[methodName], nil
	}
	return nil, fmt.Errorf(format.Alert+"Object %s don't has method \"%s\"", string(objType), methodName)
}
