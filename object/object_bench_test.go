package object

import (
	"strings"
	"testing"
)

var (
	sinkBool   bool
	sinkString string
)

func benchIntArray(n int) *Array {
	elems := make([]Object, n)
	for i := range elems {
		elems[i] = Int{Value: i}
	}
	return &Array{Elements: elems}
}

func benchStringArray(n int) *Array {
	elems := make([]Object, n)
	for i := range elems {
		elems[i] = String{Value: []rune("item-" + strings.Repeat("x", i%8))}
	}
	return &Array{Elements: elems}
}

func BenchmarkRichCompareArray100(b *testing.B) {
	var v, w Object = benchIntArray(100), benchIntArray(100)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkBool = RichCompare(v, w)
	}
}

func BenchmarkRichCompareInt(b *testing.B) {
	var v, w Object = Int{Value: 123456}, Int{Value: 123456}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkBool = RichCompare(v, w)
	}
}

func BenchmarkRichCompareString(b *testing.B) {
	var v, w Object = String{Value: []rune("hello, world")}, String{Value: []rune("hello, world")}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkBool = RichCompare(v, w)
	}
}

func BenchmarkValueEqualArray100(b *testing.B) {
	var v, w Object = benchIntArray(100), benchIntArray(100)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkBool = ValueEqual(v, w)
	}
}

func BenchmarkValueEqualString(b *testing.B) {
	var v, w Object = String{Value: []rune("hello, world")}, String{Value: []rune("hello, world")}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkBool = ValueEqual(v, w)
	}
}

func BenchmarkArrayInspect1000(b *testing.B) {
	arr := benchStringArray(1000)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkString = arr.Inspect()
	}
}

func BenchmarkToUpper(b *testing.B) {
	var s Object = String{Value: []rune("the quick brown fox jumps over the lazy dog")}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, res, _ := toUpper(s, nil)
		sinkString = res.Inspect()
	}
}
