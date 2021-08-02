package utils

import (
	"Interpreter/object"
	"crypto/md5"
	"fmt"
	"testing"
)

func TestTime33(t *testing.T) {
	fmt.Println(Time33(object.IntObj))
	fmt.Println(Time33(object.FloatObj))
	fmt.Println(Time33(object.StringObj))
	fmt.Println(Time33(object.BooleanObj))
}

func BenchmarkTime33(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Hash(object.Float{Value: 4.43})
	}
}
func BenchmarkTime332(b *testing.B) {
	h := md5.New()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.Write([]byte("你好"))
		h.Sum(nil)
	}
}
