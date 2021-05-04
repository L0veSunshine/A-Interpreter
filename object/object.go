package object

import (
	"strconv"
)

type ObjType string

type Object interface {
	Type() ObjType
	Inspect() string
}

type Number struct {
	Value float64
}

func (n Number) Type() ObjType {
	return NumberObj
}

func (n Number) Inspect() string {
	s := strconv.FormatFloat(n.Value, 'f', -1, 64)
	return s
}

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

type String struct {
	Value string
}

func (s String) Type() ObjType {
	return StringObj
}

func (s String) Inspect() string {
	return s.Value
}
