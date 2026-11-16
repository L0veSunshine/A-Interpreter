package object

import "math"

// FloorDivInt 实现 Python 风格的整数 //：向负无穷取整。
// 调用方必须保证 r != 0。
func FloorDivInt(l, r int) int {
	q := l / r
	if (l%r != 0) && ((l < 0) != (r < 0)) {
		q--
	}
	return q
}

// FloorDivFloat 实现浮点 //，结果为 floor(l/r)，类型仍是浮点。
// 调用方必须保证 r != 0。
func FloorDivFloat(l, r float64) float64 {
	return math.Floor(l / r)
}
