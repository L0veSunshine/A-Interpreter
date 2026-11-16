package main

import (
	"fmt"
	"strings"
	"testing"

	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/compiler"
	"Interpreter/ir"
	"Interpreter/lexer"
	"Interpreter/object"
	"Interpreter/parser"
	"Interpreter/vm"
)

var closurePrograms = []struct {
	name string
	src  string
	want string
}{
	{
		"escaping counter",
		`def makeCounter(n){
	def next(){
		n+=1
		return n
	}
	return next
}
var counter=makeCounter(10)
[counter(),counter(),counter()]`,
		"[11, 12, 13]",
	},
	{
		"independent invocations and slot reuse",
		`def makeCounter(n){
	def next(){
		n+=1
		return n
	}
	return next
}
var a=makeCounter(0)
var b=makeCounter(100)
[a(),b(),a(),b()]`,
		"[1, 101, 2, 102]",
	},
	{
		"siblings share a cell",
		`def makePair(){
	var x=0
	def set(n){
		x=n
		return x
	}
	def get(){
		return x
	}
	return [set,get]
}
var pair=makePair()
var set=pair[0]
var get=pair[1]
[get(),set(42),get()]`,
		"[0, 42, 42]",
	},
	{
		"outer and inner writes are visible",
		`def outer(){
	var x=1
	def change(){
		x+=4
		return x
	}
	x=10
	var first=change()
	x+=2
	return [first,x,change()]
}
outer()`,
		"[14, 16, 20]",
	},
	{
		"transitive capture across inactive frames",
		`def outer(n){
	def middle(){
		def inner(){
			n+=1
			return n
		}
		return inner
	}
	return middle
}
var middle=outer(7)
var a=middle()
var b=middle()
[a(),b(),a()]`,
		"[8, 9, 10]",
	},
	{
		"multiple lexical levels",
		`def outer(a){
	def middle(b){
		def inner(c){
			a+=1
			b+=2
			return a+b+c
		}
		return inner
	}
	return middle
}
var middle=outer(10)
var inner=middle(20)
[inner(3),inner(3)]`,
		"[36, 39]",
	},
	{
		"recursive escaped local function",
		`def makeFactorial(scale){
	def factorial(n){
		if(n<2){
			return scale
		}
		return n*factorial(n-1)
	}
	return factorial
}
var factorial=makeFactorial(2)
factorial(5)`,
		"240",
	},
	{
		"capture a sibling function",
		`def outer(n){
	def add(x){
		return n+x
	}
	def invoke(x){
		return add(x)
	}
	return invoke
}
var invoke=outer(4)
invoke(8)`,
		"12",
	},
	{
		"capture enclosing local function for recursion",
		`def make(){
	def outer(n){
		def inner(){
			return outer(n-1)
		}
		if(n==0){
			return 9
		}
		return inner()
	}
	return outer
}
var outer=make()
outer(10)`,
		"9",
	},
	{
		"mutually recursive local functions",
		`def make(){
	def even(n){
		if(n==0){
			return true
		}
		return odd(n-1)
	}
	def odd(n){
		if(n==0){
			return false
		}
		return even(n-1)
	}
	return [even,odd]
}
var pair=make()
var even=pair[0]
var odd=pair[1]
[even(20),odd(21),odd(20)]`,
		"[true, true, false]",
	},
	{
		"block local at top level",
		`var saved=none
if(true){
	var x=3
	def next(){
		x+=1
		return x
	}
	saved=next
}
[saved(),saved()]`,
		"[4, 5]",
	},
	{
		"lexical shadowing",
		`def outer(x){
	var saved=none
	if(true){
		var x=20
		def inner(){
			x+=1
			return x
		}
		saved=inner
	}
	x=100
	return [x,saved]
}
var result=outer(1)
var next=result[1]
[result[0],next(),next()]`,
		"[100, 21, 22]",
	},
	{
		"capture initialized after definition",
		`def outer(){
	def inner(){
		return x
	}
	var x=8
	return inner
}
var inner=outer()
inner()`,
		"8",
	},
	{
		"captured array methods and rebinding",
		`def outer(){
	var a=[1]
	def append(n){
		a.append(n)
		return a
	}
	def reset(){
		a=[9]
		return a
	}
	return [append,reset]
}
var pair=outer()
var push=pair[0]
var reset=pair[1]
var first=push(2)
reset()
[first,push(3)]`,
		"[[1, 2], [9, 3]]",
	},
	{
		"captured scalar method writeback",
		`def outer(){
	var s="abc"
	def upper(){
		s=s.upper()
		return s
	}
	def get(){
		return s
	}
	return [upper,get]
}
var pair=outer()
var upper=pair[0]
var get=pair[1]
[upper(),get()]`,
		"['ABC', 'ABC']",
	},
	{
		"closures as arguments and container elements",
		`def make(n){
	def add(x){
		return n+x
	}
	return add
}
def apply(fn,x){
	return fn(x)
}
var funcs=[make(2),make(5)]
var table={1:funcs[0]}
[apply(funcs[0],3),funcs[1](3),table[1](4)]`,
		"[5, 8, 6]",
	},
	{
		"closure identity",
		`def make(n){
	def get(){
		return n
	}
	return get
}
var a=make(1)
var b=make(1)
[a==a,a==b,a!=b]`,
		"[true, false, true]",
	},
	{
		"locals and frame growth",
		`def build(n){
	var x=n
	def read(){
		return x
	}
	if(n==0){
		return [read]
	}
	var result=build(n-1)
	result.append(read)
	return result
}
var funcs=build(100)
var first=funcs[0]
var last=funcs[100]
[len(funcs),first(),last()]`,
		"[101, 0, 100]",
	},
	{
		"ordinary nested functions still work",
		`def outer(n){
	def double(x){
		return x*2
	}
	return double(n)
}
outer(6)`,
		"12",
	},
}

func TestClosureSemantics(t *testing.T) {
	old := ir.FusionEnabled
	defer func() { ir.FusionEnabled = old }()
	for _, tc := range closurePrograms {
		t.Run(tc.name, func(t *testing.T) {
			for _, optimize := range []bool{false, true} {
				for _, fusion := range []bool{false, true} {
					t.Run(fmt.Sprintf("opt=%v/fusion=%v", optimize, fusion), func(t *testing.T) {
						ir.FusionEnabled = fusion
						bc := compileAlgorithm(t, tc.src, optimize)
						if err := bytecode.Validate(bc); err != nil {
							t.Fatalf("invalid bytecode: %v", err)
						}
						blob, err := bytecode.Encode(bc, bytecode.CacheMeta{Optimized: optimize})
						if err != nil {
							t.Fatal(err)
						}
						loaded, _, err := bytecode.Decode(blob)
						if err != nil {
							t.Fatal(err)
						}
						for _, program := range []*bytecode.Bytecode{bc, loaded} {
							m := vm.NewVM()
							for run := 0; run < 2; run++ {
								if err := m.Run(program); err != nil {
									t.Fatalf("run %d: %v", run, err)
								}
								if got := m.LastPop().Inspect(); got != tc.want {
									t.Fatalf("run %d: want %s, got %s", run, tc.want, got)
								}
							}
						}
					})
				}
			}
		})
	}
}

func TestClosureWithFusedLocalAccess(t *testing.T) {
	for _, tc := range closurePrograms {
		t.Run(tc.name, func(t *testing.T) {
			bc := compileAlgorithm(t, tc.src, false)
			for i, obj := range bc.Constants {
				fn, ok := obj.(object.CompiledFunc)
				if !ok {
					continue
				}
				var err error
				fn.Instructions, fn.Lines, err = code.Fuse(fn.Instructions, fn.Lines)
				if err != nil {
					t.Fatal(err)
				}
				fn.MaxStack = code.ConservativeMaxStack(fn.Instructions)
				bc.Constants[i] = fn
			}
			if err := bytecode.Validate(bc); err != nil {
				t.Fatal(err)
			}
			if got := runAlgorithm(t, bc); got != tc.want {
				t.Fatalf("want %s, got %s", tc.want, got)
			}
		})
	}
}

func TestClosureWithWideCaptureIndices(t *testing.T) {
	var src strings.Builder
	src.WriteString("def outer(){\n")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&src, "var x%d=%d\n", i, i)
	}
	src.WriteString("def inner(){\nreturn [")
	for i := 0; i < 300; i++ {
		if i > 0 {
			src.WriteString(",")
		}
		fmt.Fprintf(&src, "x%d", i)
	}
	src.WriteString("]\n}\nreturn inner\n}\nvar inner=outer()\nvar result=inner()\n[len(result),result[0],result[299]]")
	for _, optimize := range []bool{false, true} {
		bc := compileAlgorithm(t, src.String(), optimize)
		blob, err := bytecode.Encode(bc, bytecode.CacheMeta{})
		if err != nil {
			t.Fatal(err)
		}
		loaded, _, err := bytecode.Decode(blob)
		if err != nil {
			t.Fatal(err)
		}
		if got := runAlgorithm(t, loaded); got != "[300, 0, 299]" {
			t.Fatalf("optimize=%v: got %s", optimize, got)
		}
	}
}

func TestClosureRuntimeErrors(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`def make(n){
	def inner(x){
		return n+x
	}

	return inner
}
var inner=make(1)
inner()`, "wrong number of arguments"},
		{`def make(n){
	def inner(){
		return n/0
	}
	return inner
}
var inner=make(1)
inner()`, "inner: line 3: division by zero"},
	}
	for _, tc := range cases {
		for _, optimize := range []bool{false, true} {
			bc := compileAlgorithm(t, tc.src, optimize)
			m := vm.NewVM()
			if err := m.Run(bc); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("optimize=%v: want error containing %q, got %v", optimize, tc.want, err)
			}
		}
	}
}

func TestClosureOptimizationIsFunctionLocal(t *testing.T) {
	const src = `def make(){
	var n=0
	def next(){
		n+=1
		return n
	}
	return next
}
def hot(n){
	return (2+3)*n
}
var next=make()
[next(),hot(7)]`
	old := ir.FusionEnabled
	ir.FusionEnabled = false
	defer func() { ir.FusionEnabled = old }()
	p := parser.NewParser(lexer.NewLexer(src))
	node := p.Parse()
	if p.HasError() {
		t.Fatal(p.Errs())
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.Compile(node)
	if c.HasError() {
		t.Fatal(c.Errs())
	}
	if err := c.OptimizeErr(); err == nil || !strings.Contains(err.Error(), "closure cells") {
		t.Fatalf("expected explicit closure optimization diagnostic, got %v", err)
	}
	bc := c.ByteCode()
	found := false
	for _, obj := range bc.Constants {
		fn, ok := obj.(object.CompiledFunc)
		if !ok || fn.FnName != "hot" {
			continue
		}
		found = true
		for i := 0; i < len(fn.Instructions); {
			op := code.Opcode(fn.Instructions[i])
			if op == code.OpAdd {
				t.Fatal("closure fallback prevented another function's constant folding")
			}
			_, width := code.ReadOperand(code.Definitions[op], fn.Instructions[i+1:])
			i += 1 + width
		}
	}
	if !found {
		t.Fatal("hot function missing")
	}
	if got := runAlgorithm(t, bc); got != "[1, 35]" {
		t.Fatalf("got %s", got)
	}
}
