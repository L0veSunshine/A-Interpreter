package object

import "math"

// ValueEqual 与 RichCompare 判定结果一致，但对 Int/Boolean/String/Null/Array/Map
// 直接比较值，不做字符串格式化。
//
// Float 与其它类型仍走 RichCompare：Float 的 Inspect 有精度截断（>1e13 只保留
// 12 位有效数字，-0 与 0 的文本不同），按值比较会改变既有语义。
func ValueEqual(v, w Object) bool {
	switch a := v.(type) {
	case Int:
		b, ok := w.(Int)
		return ok && a.Value == b.Value
	case Boolean:
		b, ok := w.(Boolean)
		return ok && a.Value == b.Value
	case String:
		b, ok := w.(String)
		return ok && runesEqual(a.Value, b.Value)
	case Null:
		_, ok := w.(Null)
		return ok
	case *Closure:
		b, ok := w.(*Closure)
		return ok && a == b
	case *Array:
		b, ok := w.(*Array)
		if !ok || len(a.Elements) != len(b.Elements) {
			return false
		}
		for i := range a.Elements {
			if !ValueEqual(a.Elements[i], b.Elements[i]) {
				return false
			}
		}
		return true
	case Map:
		b, ok := w.(Map)
		if !ok || len(a.Store) != len(b.Store) {
			return false
		}
		for h, p := range a.Store {
			q, ok := b.Store[h]
			if !ok || !ValueEqual(p.Key, q.Key) || !ValueEqual(p.Item, q.Item) {
				return false
			}
		}
		return true
	}
	return RichCompare(v, w)
}

func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Order 是数值三路比较的结果。
type Order int

const (
	Less      Order = -1
	Equal     Order = 0
	Greater   Order = 1
	Unordered Order = 2 // 与 NaN 比较：既不相等也不有序
)

// CompareNumeric 精确比较两个数值对象（Int/Float 的任意组合）。
//
// 不能一律转成 float64：float64 只有 53 位尾数，2^53 以上相邻的整数会被
// 判成相等（9007199254740993 == 9007199254740992）。int/int 直接按整数比较，
// int/float 把浮点拆成整数部分与小数部分比较，全程不分配。
//
// 第二个返回值表示两边是否都是数值；为 false 时调用方应走对象比较。
func CompareNumeric(left, right Object) (Order, bool) {
	switch l := left.(type) {
	case Int:
		switch r := right.(type) {
		case Int:
			return compareInts(l.Value, r.Value), true
		case Float:
			return compareIntToFloat(l.Value, r.Value), true
		}
	case Float:
		switch r := right.(type) {
		case Int:
			return -compareIntToFloat(r.Value, l.Value), true
		case Float:
			return compareFloats(l.Value, r.Value), true
		}
	}
	return Equal, false
}

func compareInts(l, r int) Order {
	switch {
	case l < r:
		return Less
	case l > r:
		return Greater
	default:
		return Equal
	}
}

func compareFloats(l, r float64) Order {
	switch {
	case math.IsNaN(l) || math.IsNaN(r):
		return Unordered
	case l < r:
		return Less
	case l > r:
		return Greater
	default:
		return Equal
	}
}

// compareIntToFloat 精确比较 int 与 float64。
//
// 先把浮点拆成整数部分（trunc）与小数部分：整数部分能精确落进 int 时按整数
// 比较，剩下的小数部分决定次序。NaN 单独标记，±Inf 由范围检查自然处理。
func compareIntToFloat(i int, f float64) Order {
	if math.IsNaN(f) {
		return Unordered
	}
	trunc := math.Trunc(f)
	// 2^63 已经超出 int 能表示的范围（float 在这个量级上都是整数）
	if trunc >= 9223372036854775808.0 {
		return Less
	}
	if trunc < -9223372036854775808.0 {
		return Greater
	}
	whole := int(trunc)
	if i != whole {
		return compareInts(i, whole)
	}
	switch fraction := f - trunc; {
	case fraction > 0:
		return Less
	case fraction < 0:
		return Greater
	default:
		return Equal
	}
}
