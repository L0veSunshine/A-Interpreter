package object

import "strconv"

type Boolean struct {
	Value bool
}

func (b Boolean) Type() ObjType {
	return BooleanObj
}

func (b Boolean) Inspect() string {
	s := strconv.FormatBool(b.Value)
	return s
}
