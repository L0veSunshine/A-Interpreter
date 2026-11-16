package object

import (
	"fmt"

	"Interpreter/format"
)

// MethodKind 是"带方法表的对象类别"。
//
// 用类别而不是 string 类型的 ObjType 做下标，是为了让 `a.append(x)` 的
// 方法分派变成"一次数组下标"而不是"一次字符串比较 + 一次 map 查找"。
type MethodKind uint8

const (
	KindArray MethodKind = iota
	KindString
	// NumMethodKinds 是类别总数，同时是方法表的行数。
	NumMethodKinds
)

// methodLists 与 MethodKind 一一对应。
var methodLists = [NumMethodKinds]ObjMethods{
	ArrayMethodList,
	StringMethodList,
}

// KindOf 把对象类型映射成方法表类别。
func KindOf(t ObjType) (MethodKind, bool) {
	switch t {
	case ArrayObj:
		return KindArray, true
	case StringObj:
		return KindString, true
	}
	return 0, false
}

// MethodTables 是"方法名下标 -> 具体实现"的表，按对象类别分行。
//
// 下标来自符号表（parser.MethodNames），所以同一份字节码里同一个方法名
// 永远对应同一个下标；表在 VM 执行前构建一次，之后每次调用只做下标访问。
type MethodTables [NumMethodKinds][]ObjGOFunc

// BuildMethodTables 按符号表里的方法名顺序构建查找表。
// 某个类别没有该方法时对应位置是 nil，由调用方报"don't has method"。
func BuildMethodTables(names []string) MethodTables {
	var t MethodTables
	for kind := range t {
		t[kind] = make([]ObjGOFunc, len(names))
		for i, name := range names {
			if m, ok := methodLists[kind][name]; ok {
				t[kind][i] = m.M
			}
		}
	}
	return t
}

// Lookup 返回某个类别下第 idx 个方法的实现；不存在时返回 nil。
func (t *MethodTables) Lookup(kind MethodKind, idx int) ObjGOFunc {
	if kind >= NumMethodKinds || idx < 0 || idx >= len(t[kind]) {
		return nil
	}
	return t[kind][idx]
}

// MissingMethodError 是"对象没有这个方法"的统一错误信息。
func MissingMethodError(objType ObjType, methodName string) error {
	return fmt.Errorf(format.Alert+"Object %s don't has method %s", string(objType), methodName)
}
