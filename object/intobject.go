package object

import "strconv"

type Int struct {
	Value int
}

func (n Int) Type() ObjType {
	return IntObj
}

func (n Int) Inspect() string {
	s := strconv.Itoa(n.Value)
	return s
}
