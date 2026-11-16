package ir_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/ir"
	"Interpreter/object"
	"Interpreter/vm"
)

func checkMethodProgram(t *testing.T, src, want, wantErr string) {
	t.Helper()
	raw := compileSrc(t, src)
	baseline, baselineErr := runBC(raw)
	if wantErr == "" {
		if baselineErr != nil || baseline != want {
			t.Fatalf("unoptimized: want %q, got %q, err=%v", want, baseline, baselineErr)
		}
	} else if baselineErr == nil || !strings.Contains(baselineErr.Error(), wantErr) {
		t.Fatalf("unoptimized: want error containing %q, got %v", wantErr, baselineErr)
	}

	for _, fusion := range []bool{false, true} {
		t.Run(fmt.Sprintf("fusion=%v", fusion), func(t *testing.T) {
			previous := ir.FusionEnabled
			ir.FusionEnabled = fusion
			defer func() { ir.FusionEnabled = previous }()
			optimized, err := ir.Optimize(raw)
			if err != nil {
				t.Fatalf("optimizer must not silently fall back: %v", err)
			}
			if err := bytecode.Validate(optimized); err != nil {
				t.Fatalf("invalid optimized bytecode: %v", err)
			}
			stackBound, err := code.MaxStack(optimized.Instruction)
			if err != nil || stackBound != optimized.MainMaxStack {
				t.Fatalf("main stack bound: stored=%d actual=%d err=%v",
					optimized.MainMaxStack, stackBound, err)
			}
			hasFunction := false
			for _, constant := range optimized.Constants {
				if fn, ok := constant.(object.CompiledFunc); ok {
					hasFunction = true
					bound, err := code.MaxStack(fn.Instructions)
					if err != nil || bound != fn.MaxStack {
						t.Fatalf("%s stack bound: stored=%d actual=%d err=%v",
							fn.FnName, fn.MaxStack, bound, err)
					}
				}
			}
			encoded, err := bytecode.Encode(optimized, bytecode.CacheMeta{Optimized: true})
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			cached, _, err := bytecode.Decode(encoded)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			for _, bc := range []*bytecode.Bytecode{optimized, cached} {
				m := vm.NewVM()
				runErr := m.Run(bc)
				if baselineErr != nil {
					if runErr == nil || runErr.Error() != baselineErr.Error() {
						t.Fatalf("error changed:\nraw: %v\noptimized: %v", baselineErr, runErr)
					}
					continue
				}
				if runErr != nil {
					t.Fatalf("run: %v", runErr)
				}
				if got := m.LastPop().Inspect(); got != want {
					t.Fatalf("want %q, got %q", want, got)
				}
				if m.LocalsDepth() != bc.MainLocals {
					t.Fatalf("locals leaked: got %d, main owns %d", m.LocalsDepth(), bc.MainLocals)
				}
				if !hasFunction && m.MaxStackDepth() > bc.MainMaxStack {
					t.Fatalf("operand stack exceeded bound: peak=%d bound=%d", m.MaxStackDepth(), bc.MainMaxStack)
				}
			}
		})
	}
}

func TestMethodCallRegression(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"pop result and receiver", `def f(){
	var a=[10,20,30]
	var x=a.pop()
	return [x,len(a),a[1]]
}
f()`, "[30, 2, 20]"},
		{"string distinct results", `def f(){
	var s="Ab"
	var upper=s.upper()
	return [s,upper]
}
f()`, "['Ab', 'AB']"},
		{"split return and receiver", `def f(){
	var s="a:b:c"
	var parts=s.split(":")
	return [s,parts]
}
f()`, "['a:b:c', ['a', 'b', 'c']]"},
		{"return method result", `def f(){
	var a=[1,7]
	return a.pop()
}
f()`, "7"},
		{"comparison result after method", `var n=0
def f(){
	var a=[1]
	a.pop()
	n+=1
	return n<4
}
[f(),f(),f(),f()]`, "[true, true, true, false]"},
		{"nested calls same receiver", `def f(){
	var a=[1,2,3]
	a.append(a.pop())
	return a
}
f()`, "[1, 2, 3]"}, // 引用语义：接收者即同一个对象，pop 之后 append 作用在缩短后的数组上
		{"argument evaluation order", `var trace=0
def mark(n){
	trace=trace*10+n
	return n
}
var a=[]
a.append(mark(1),mark(2),mark(3))
[trace,a]`, "[123, [1, 2, 3]]"},
		{"receiver evaluated before argument", `var trace=0
def receiver(){
	trace=trace*10+1
	return []
}
def arg(){
	trace=trace*10+2
	return 8
}
receiver().append(arg())
trace`, "12"},
		{"read before reverse", `def f(){
	var a=[1,2,3]
	var x=a[0]
	a.reverse()
	return [x,a[0]]
}
f()`, "[1, 3]"},
		{"dependent read before reverse", `def f(){
	var a=[1,2,3]
	var x=a[0]+10
	a.reverse()
	return [x,a[0]]
}
f()`, "[11, 3]"},
		{"index CSE barrier", `def f(){
	var a=[1,2]
	var x=a[0]+a[0]
	a.reverse()
	return x+a[0]+a[0]
}
f()`, "6"},
		{"alias mutation", `def f(){
	var a=[1,2]
	var b=a
	var x=b[0]
	a.reverse()
	return [x,b[0],a[0]]
}
f()`, "[1, 2, 2]"},
		{"pop in branch phi", `def f(flag){
	var a=[1,2,3]
	var x=0
	if(flag){
		x=a.pop()
	}else{
		a.append(4)
		x=a.pop()
	}
	return [x,len(a)]
}
[f(true),f(false)]`, "[[3, 2], [4, 3]]"},
		{"loop carried receiver and result", `def f(){
	var a=[]
	var sum=0
	for(var i=0;i<50;i=i+1){
		a.append(i)
		if(i%3==0){
			sum=sum+a.pop()
		}
	}
	return [len(a),sum,a[0],a[len(a)-1]]
}
f()`, "[33, 408, 1, 49]"},
		{"nested loop break", `def f(){
	var a=[]
	for(var i=0;i<4;i=i+1){
		for(var j=0;j<4;j=j+1){
			a.append(i*10+j)
			if(j==1){break}
		}
	}
	return a
}
f()`, "[0, 1, 10, 11, 20, 21, 30, 31]"},
		{"short circuit skips method", `var a=[1,2]
false and a.pop()
true or a.pop()
a`, "[1, 2]"},
		{"short circuit evaluates selected methods", `def f(){
	var a=[1,2,3]
	var x=true and a.pop()
	var y=false or a.pop()
	return [x,y,a]
}
f()`, "[true, true, [1]]"},
		{"global write through call", `var a=[]
def add(n){
	a.append(n)
	return len(a)
}
var x=add(7)
var y=add(8)
[x,y,a]`, "[1, 2, [7, 8]]"},
		{"recursive methods", `def f(n){
	var a=[n]
	if(n>0){
		a.append(f(n-1))
	}
	return a
}
f(3)`, "[3, [2, [1, [0]]]]"},
		{"discarded result still mutates", `def f(){
	var a=[1,2,3]
	a.pop()
	a.reverse()
	return a
}
f()`, "[2, 1]"},
		{"expression receiver last pop", `[1,2].pop()`, "2"},
		{"empty append", `var a=[1]
a.append()
a`, "[1]"},
		{"parameter receiver ABI", `def f(a,n){
	var old=n
	a.append(n)
	var x=a.pop()
	return [a,x,old,n]
}
f([1],7)`, "[[1], 7, 7, 7]"},
		{"two live receivers", `def f(flag){
	var a=[1,2]
	var b=[3,4]
	var x=a.pop()
	var y=b.pop()
	if(flag){
		a.append(y)
		b.append(x)
	}
	return [a,b,x,y]
}
[f(true),f(false)]`, "[[[1, 4], [3, 2], 2, 4], [[1], [3], 2, 4]]"},
		{"loop phi swaps method results", `def f(){
	var a=[1,2]
	var b=[3,4]
	var x=0
	var y=0
	for(var i=0;i<5;i=i+1){
		x=a.pop()
		y=b.pop()
		a.append(y)
		b.append(x)
	}
	return [a,b,x,y]
}
f()`, "[[1, 4], [3, 2], 2, 4]"},
		{"main loop does not leak stack", `var a=[]
for(var i=0;i<5000;i=i+1){
	a.append(i)
	a.pop()
}
len(a)`, "0"},
		{"unused local method results preserve lastPop", `42
def f(){
	var s="ab"
	var unused=s.upper()
}
var ignored=f()`, "42"},
		{"unused pop result preserves lastPop", `42
def f(){
	var a=[1,2]
	var unused=a.pop()
}
var ignored=f()`, "42"},
		{"pop result survives later call", `def f(){
	var a=[1,2,3]
	var x=a.pop()
	a.reverse()
	return [x,a.pop(),a]
}
f()`, "[3, 1, [2]]"},
		{"pop results preserve left to right", `def f(){
	var a=[1,2,3,4]
	var x=a.pop()*10+a.pop()
	return [x,a]
}
f()`, "[43, [1, 2]]"},
		{"argument mutates indexed receiver", `def f(){
	var a=[[1],[2]]
	a[0].append(a.pop())
	return a
}
f()`, "[[1, [2]]]"}, // 引用语义：a[0] 与参数求值期间被 pop 修改的是同一批对象
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkMethodProgram(t, tc.src, tc.want, "")
		})
	}
}

func TestMethodCallErrorsPreserveOrderAndLines(t *testing.T) {
	cases := []struct {
		name, src, wantErr string
	}{
		{"empty pop", "def f(){\nvar a=[]\na.pop()\n}\nf()", "Index"},
		{"missing method", "var a=[1]\na.missing()", "missing"},
		{"unsupported receiver", "var a=1\na.append(2)", "method"},
		{"split type", "var s=\"a:b\"\ns.split(1)", "TypeError"},
		{"split arity", "var s=\"a:b\"\ns.split()", "arg"},
		{"upper arity", "var s=\"ab\"\ns.upper(1)", "arg"},
		{"argument error first", "var s=\"ab\"\ns.missing(1/0)", "division by zero"},
		{"read error before mutation", "var a=[1]\nvar x=a[9]\na.reverse()\nx", "index"},
		{"computation error before method", "var a=[]\nvar x=1/0\na.pop()\nx", "division by zero"},
		{"unused method result error", "def f(){\nvar a=[]\nvar unused=a.pop()\nreturn 1\n}\nf()", "Index"},
		{"argument value original error line", "def f(){\nvar a=[]\nvar x=1/0\na.append(x)\n}\nf()", "division by zero"},
		{"receiver value original error line", "def f(){\nvar a=[]\nvar x=a[3]\nx.append(1)\n}\nf()", "index"},
		{"binary error precedes pop", "def f(){\nvar a=[]\nreturn 1/0+a.pop()\n}\nf()", "division by zero"},
		{"argument dependency before reverse", "def f(){\nvar a=[1]\nvar x=a[3]+1\na.reverse()\na.append(x)\n}\nf()", "index"},
		{"method line restored after argument", "def f(){\nvar s=\"ab\"\nvar x=1+2\ns.upper(x)\n}\nf()", "arg"},
		{"nested argument call error line", "def bad(){\nreturn 1/0\n}\nvar a=[]\na.append(bad())", "division by zero"},
		{"indexed argument before missing method", "var a=[1]\na.missing(a[9])", "index"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkMethodProgram(t, tc.src, "", tc.wantErr)
		})
	}
}

func TestMethodCallSeededMutations(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for program := 0; program < 25; program++ {
		var src strings.Builder
		src.WriteString("def f(){\nvar a=[]\nvar total=0\n")
		var values []int
		total := 0
		for step := 0; step < 30; step++ {
			switch choice := rng.Intn(4); {
			case len(values) == 0 || choice == 0:
				v := rng.Intn(100)
				fmt.Fprintf(&src, "a.append(%d)\n", v)
				values = append(values, v)
			case choice == 1:
				src.WriteString("total=total+a.pop()\n")
				total += values[len(values)-1]
				values = values[:len(values)-1]
			case choice == 2:
				src.WriteString("a.reverse()\n")
				for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
					values[i], values[j] = values[j], values[i]
				}
			default:
				fmt.Fprintf(&src, "total=total+a[%d]\n", len(values)-1)
				total += values[len(values)-1]
			}
		}

		src.WriteString("return [total,len(a)]\n}\nf()")
		t.Run(fmt.Sprintf("program-%02d", program), func(t *testing.T) {
			checkMethodProgram(t, src.String(), fmt.Sprintf("[%d, %d]", total, len(values)), "")
		})
	}
}

func TestMethodCallSeededLoopBranches(t *testing.T) {
	rng := rand.New(rand.NewSource(314159))
	for program := 0; program < 15; program++ {
		n, divisor := 20+rng.Intn(40), 2+rng.Intn(5)
		src := fmt.Sprintf(`def f(n){
	var a=[]
	var total=0
	for(var i=0;i<n;i=i+1){
		a.append(i)
		if(i%%%d==0){
			total=total+a.pop()
		}else{
			a.reverse()
			total=total+a[0]
		}
	}
	return [total,len(a),a]
}
f(%d)`, divisor, n)
		var values []int
		total := 0
		for i := 0; i < n; i++ {
			values = append(values, i)
			if i%divisor == 0 {
				total += values[len(values)-1]
				values = values[:len(values)-1]
			} else {
				for l, r := 0, len(values)-1; l < r; l, r = l+1, r-1 {
					values[l], values[r] = values[r], values[l]
				}
				total += values[0]
			}
		}
		var items []string
		for _, v := range values {
			items = append(items, fmt.Sprint(v))
		}
		want := fmt.Sprintf("[%d, %d, [%s]]", total, len(values), strings.Join(items, ", "))
		t.Run(fmt.Sprintf("program-%02d", program), func(t *testing.T) {
			checkMethodProgram(t, src, want, "")
		})
	}
}

func TestMethodCallCrossBlockArgumentFallsBack(t *testing.T) {
	raw := compileSrc(t, `var a=[]
a.append(false and 1/0)
a.append(true or 1/0)
a`)
	optimized, err := ir.Optimize(raw)
	if err == nil || !strings.Contains(err.Error(), "value flows across block boundary") {
		t.Fatalf("expected explicit stack-across-block fallback, got %v", err)
	}
	for _, bc := range []*bytecode.Bytecode{raw, optimized} {
		got, err := runBC(bc)
		if err != nil || got != "[false, true]" {
			t.Fatalf("fallback changed short circuit behavior: got=%q err=%v", got, err)
		}
	}
}
