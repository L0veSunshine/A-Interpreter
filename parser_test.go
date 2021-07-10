package main

import (
	"Interpreter/compiler"
	"Interpreter/object"
	vm2 "Interpreter/vm"
	"fmt"
	"strconv"
	"testing"
	"time"
)

var s = `var t=10/2-9*8*2
print(t)`
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
print(sum)`

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

var s12 = `
var a=1
var b=0
for(a<=101){
	b=b+a
	a=a+1
}
b`

var s13 = `
var t=15
var i=10/2
var const=0
var tmp=9999
for(tmp>0.000000001){
i=i-(i**2-t)/(2*i)
const=const+1
if(i**2-t>0){
tmp=i**2-t}else{
tmp=-(i**2-t)}
}
i`

var s14 = `true ==(1<=-1)`

var s15 = `
def sqrt(t) {
var i=10/2
var const=0
var tmp=9999
for(tmp>0.000000001){
i=i-(i**2-t)/(2*i)
const=const+1
if(i**2-t>0){
tmp=i**2-t}else{
tmp=-(i**2-t)}
}
return i}
var a=0
var x=0
for (a<1000){
sqrt(sqrt(4)+sqrt(sqrt(1)+sqrt(4)))
a=a+1}
`

var s16 = `
var idx=0
var s=""
for(idx<10){
s=s+"你好"
print(s)
idx=idx+1
}`

var s17 = `
def sqrt(t) {
var i=10/2
var const=0
var tmp=9999
for(tmp>0.000000001){
i=i-(i**2-t)/(2*i)
const=const+1
if(i**2-t>0){
tmp=i**2-t}else{
tmp=-(i**2-t)}
}
return i}
var a=0
var res=0
for (a<2){
res=sqrt(sqrt(4)+sqrt(9))
a=a+1}
if(res>=2){
res=res+10}`

var s18 = `
var start=30
def fib(x){
if(x<=1){
return x}else{
return fib(x-1)+fib(x-2)}
}
start=fib(start)
print(start)`

var s19 = `(1>2)==false`

func TestParser_Parse(t *testing.T) {
	st := time.Now()
	lex := NewLexer(s19)
	fmt.Println(lex.Array())
	p := NewParser(lex)
	ast := p.Parse()
	if !p.HasError() {
		fmt.Println(ast.Str())
	} else {
		fmt.Println(p.Errs(), len(p.Errs()))
	}
	c := compiler.NewCompiler()
	c.SetMode()
	c.Compile(ast)
	c.Debug()
	st1 := time.Now()
	vm := vm2.NewVM()
	err := vm.Run(c.ByteCode())
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Compile Time:" + strconv.FormatFloat(st1.Sub(st).Seconds(), 'f', -1, 32))
	fmt.Println("Run Time:" + strconv.FormatFloat(time.Since(st1).Seconds(), 'f', -1, 32))
	//fmt.Println(vm.LastPop().Inspect())
}
func BenchmarkExec(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	lex := NewLexer(s18)
	p := NewParser(lex)
	ast := p.Parse()
	//if !p.HasError() {
	//	fmt.Println(ast.Str())
	//} else {
	//	fmt.Println(p.errs, len(p.errs))
	//}
	c := compiler.NewCompiler()
	c.Compile(ast)
	//c.ByteCode()
	vm := vm2.NewVM()
	var err error
	for i := 0; i < b.N; i++ {
		err = vm.Run(c.ByteCode())
		if err != nil {
			fmt.Println(err)
		}
	}
	//fmt.Println(vm.LastPop().Inspect())
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

func BenchmarkName1(b *testing.B) {
	var ls [10]object.Object
	var sls []object.Object
	sls = append(sls, object.Int{Value: 1})
	ls[0] = object.Int{Value: 1}
	ls[1] = object.Int{Value: 2}
	var obj object.Object
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		//*(*uint64)(x) = val
		obj = sls[0]
		//obj = *(*object.Object)(unsafe.Pointer(&ls))
	}
	fmt.Println(obj.Inspect())
}

var ori = `def add(a,b){
var z=100
var c=1
c=c-a-b
z=z-1
return c}
var x=100
var a=1
var b=1
var c=1
var r=add(2,3)
x=a+1
x=x*r
x`

var ori1 = `
def sqrt(t) {
var i=10/2
var const=0
var tmp=9999
for(tmp>0.000000001){
i=i-(i**2-t)/(2*i)
const=const+1
if(i**2-t>0){
tmp=i**2-t}else{
tmp=-(i**2-t)}
}
return i}
var r=sqrt(sqrt(4)+sqrt(9))
r`

func TestParseParams(t *testing.T) {
	lex := NewLexer(ori1)
	p := NewParser(lex)
	ast := p.Parse()
	fmt.Println(ast.Str())
	c := compiler.NewCompiler()
	c.Compile(ast)
	c.Debug()
	vm := vm2.NewVM()
	if err := vm.Run(c.ByteCode()); err != nil {
		fmt.Println(err)
	}
	//fmt.Println(vm.LastPop().Inspect())
}
