package object

import (
	"fmt"
)

type ObjType string

type Object interface {
	Type() ObjType
	Inspect() string
}

type Number struct {
	Value float64
}

func (n *Number) Type() ObjType {
	return NumberObj
}

func (n *Number) Inspect() string {
	return fmt.Sprint(n.Value)
}
