package object

import (
	"strconv"
)

type ObjType string

type Object interface {
	Type() ObjType
	Inspect() string
}

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

type Float struct {
	Value float64
}

func (f Float) Type() ObjType {
	return FloatObj
}

func (f Float) Inspect() string {
	s := strconv.FormatFloat(f.Value, 'f', -1, 64)
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

type Null struct {
}

func (n Null) Type() ObjType {
	return NullObj
}

func (n Null) Inspect() string {
	return ""
}
