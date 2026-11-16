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
		// 64 位精度：32 位会把 1/3 显示成 0.33333334（值仍是 float64）
		s = strconv.FormatFloat(f.Value, 'f', -1, 64)
	}
	return s
}
