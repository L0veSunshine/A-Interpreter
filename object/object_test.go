package object

import (
	"fmt"
	"testing"
	"time"
	"unsafe"
)

type TestObject interface {
	Val() float64
	ObjType() string
}

type obj struct {
	cnt     int
	objType string
}

type floatObj struct {
	cnt     int
	objType string
	val     float64
}

type floatObjInterface struct {
	val float64
}

func (i floatObjInterface) Val() float64 {
	return i.val
}

func (i floatObjInterface) ObjType() string {
	return "Float"
}

func TestC(t *testing.T) {
	var fo = &floatObj{
		cnt:     5,
		objType: "Float",
		val:     10,
	}
	var c *obj
	var count = 0
	st := time.Now()
	for count < 1000000000 {
		c = (*obj)(unsafe.Pointer(fo))
		count += 1
	}
	et := time.Since(st).Milliseconds()
	fmt.Println(float64(et) / 1000000000)
	ori := (*floatObj)(unsafe.Pointer(c))
	fmt.Println(ori.val)
}

func BenchmarkPointConvert(b *testing.B) {
	b.ReportAllocs()
	var fo = &floatObj{
		cnt:     5,
		objType: "Float",
		val:     10,
	}
	var c = (*obj)(unsafe.Pointer(fo))
	var fc *floatObj
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fc = (*floatObj)(unsafe.Pointer(c))
	}
	fmt.Println(fc.val)
}

func BenchmarkInterfaceConvert(b *testing.B) {
	b.ReportAllocs()
	var io = floatObjInterface{
		val: 10,
	}
	var inter = TestObject(io)
	var intO floatObjInterface
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		intO = inter.(floatObjInterface)
	}
	fmt.Println(intO.val)
}
