package object

import "strconv"

type Float struct {
	Value float64
}

func (f Float) Type() ObjType {
	return FloatObj
}

func (f Float) Inspect() string {
	var s string
	if f.Value > 10e12 {
		s = strconv.FormatFloat(f.Value, 'e', 12, 64)
	} else {
		s = strconv.FormatFloat(f.Value, 'f', -1, 32)
	}
	return s
}
