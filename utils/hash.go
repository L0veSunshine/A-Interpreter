package utils

import (
	"Interpreter/object"
	"fmt"
	"strconv"
)

func Time33(str string) int {
	rs := []rune(str)
	hash := 5381
	length := len(rs)
	for i := 0; i < length; i++ {
		hash += hash<<5 + int(rs[i])
	}
	return hash & 0x7FFFFFFF
}

func Hash(obj object.Object) int {
	var res int
	switch obj.Type() {
	case object.IntObj:
		iVal, err := strconv.Atoi(obj.Inspect())
		if err != nil {
			fmt.Println(err)
		}
		res = iVal + 193460240 //time33("IntObj")
	case object.FloatObj:
		res = Time33(obj.Inspect()) + 221172091
	case object.StringObj:
		res = Time33(obj.Inspect()) + 1374591964
	case object.BooleanObj:
		res = Time33(obj.Inspect()) + 1732606053
	default:
		res = 0
	}
	return res
}
