package utils

import (
	"Interpreter/object"
	"testing"
)

var sinkHash int

func BenchmarkHashString(b *testing.B) {
	var key object.Object = object.String{Value: []rune("some_dictionary_key")}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkHash = Hash(key)
	}
}

func BenchmarkHashInt(b *testing.B) {
	var key object.Object = object.Int{Value: 987654}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkHash = Hash(key)
	}
}
