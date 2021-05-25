package main

import (
	"fmt"
	"testing"
	"time"
)

var s = `10/2-9*8*2`
var s1 = `{var a=1
var b=a+3
b=b+2*a
c=-1+2
d=1<=2
return 6*a}
`
var s2 = `if (a<=3)
{a=a+3}`

var s3 = `
var a=1
a=a+1
for (a<=1)
{
a=a+1
b=1
if (b==1)
{
b=b+1
for (b<=10){
b=b+0.5}
}
}
if(a<b){for(i>=10){
i=i-1}
}else{
b=1}
`
var s8 = `1+2+3`

var s4 = `"hello "+"world"`

var s5 = `false and not true`

var s6 = `if (1<2){10}`

var s7 = `for(1>2){10}`

var s9 = `var a=9
var b=10
var c=11
a=a+1
a=b+a+5+c`

var s10 = `var a=1
var sum=0
for(a<=10000000){
if(a%3==1){
sum=sum+a}
a=a+1}
sum`

var s11 = `var a=2
var sum=0
for(a<10000){
var count=0
var b=2
for(b<a){
if(a%b==0){
count=count+1}
b=b+1}
if(count==0){
sum=sum+a}
a=a+1}
sum`

func TestParser_Parse(t *testing.T) {
	st := time.Now()
	lex := NewLexer(s11)
	p := NewParser(lex)
	ast := p.Parse()
	if !p.HasError() {
		fmt.Println(ast.Str())
	} else {
		fmt.Println(p.errs, len(p.errs))
	}
	c := NewCompiler()
	c.Compile(ast)
	c.Debug()
	vm := NewVM()
	err := vm.Run(c.ByteCode())
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(time.Since(st).Seconds())
	fmt.Println(vm.LastPop().Inspect())
}
func BenchmarkExec(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	lex := NewLexer(s11)
	p := NewParser(lex)
	ast := p.Parse()
	//if !p.HasError() {
	//	fmt.Println(ast.Str())
	//} else {
	//	fmt.Println(p.errs, len(p.errs))
	//}
	c := NewCompiler()
	c.Compile(ast)
	//c.Debug()
	vm := NewVM()
	var err error
	for i := 0; i < b.N; i++ {
		err = vm.Run(c.ByteCode())
		if err != nil {
			fmt.Println(err)
		}
	}
	fmt.Println(vm.LastPop().Inspect())
}

func BenchmarkParser_Parse(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex := NewLexer(s3)
		p := NewParser(lex)
		p.Parse()
	}
}

func Test1(t *testing.T) {
	a := []int{1, 2, 3, 4}
	b := []int{3, 4, 5, 6}
	a = append(a, b...)
	fmt.Println(a)
}

func TestAsz(t *testing.T) {
	sum := 0
	for i := 2; i < 10000; i++ {
		count := 0
		for j := 2; j < i; j++ {
			if i%j == 0 {
				count++
			}
		}
		if count == 0 {
			sum += i
		}
	}
	fmt.Println(sum)
}
