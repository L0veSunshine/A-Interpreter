package utils

import (
	"Interpreter/object"
)

func Time33(str string) int {
	hash := 5381
	for _, r := range str {
		hash += hash<<5 + int(r)
	}
	return hash & 0x7FFFFFFF
}

// time33Runes 与 Time33 对同一段文本的结果相同，但直接消费 []rune，避免
// String -> string -> []rune 两次分配。
func time33Runes(rs []rune) int {
	hash := 5381
	for _, r := range rs {
		hash += hash<<5 + int(r)
	}
	return hash & 0x7FFFFFFF
}

func Hash(obj object.Object) int {
	switch o := obj.(type) {
	case object.Int:
		return o.Value + 193460240 //time33("IntObj")
	case object.Float:
		return Time33(o.Inspect()) + 221172091
	case object.String:
		return time33Runes(o.Value) + 1374591964
	case object.Boolean:
		return Time33(o.Inspect()) + 1732606053
	default:
		return 0
	}
}
