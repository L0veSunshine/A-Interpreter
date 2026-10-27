package ir_test

import (
	"Interpreter/bytecode"
	"Interpreter/cfg"
	"Interpreter/compiler"
	"Interpreter/ir"
	"Interpreter/lexer"
	"Interpreter/object"
	"Interpreter/parser"
	"Interpreter/vm"
	"strings"
	"testing"
)

// compileSrc 走完整前端，得到未优化的字节码。
func compileSrc(t *testing.T, src string) *bytecode.Bytecode {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(src))
	node := p.Parse()
	if p.HasError() {
		t.Fatalf("parse error: %v", p.Errs())
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.SetOptimize(false)
	c.Compile(node)
	if c.HasError() {
		t.Fatalf("compile error: %v", c.Errs())
	}
	return c.ByteCode()
}

func runBC(bc *bytecode.Bytecode) (string, error) {
	m := vm.NewVM()
	if err := m.Run(bc); err != nil {
		return "", err
	}
	return m.LastPop().Inspect(), nil
}

// program 是一组必须"优化前后行为一致"的用例。
var corpus = []struct {
	name string
	src  string
}{
	{"arith", "9/2-9*8.102*2"},
	{"fold", "1+2*3"},
	{"assign", "var a=1\na=a+1\na"},
	{"if-else", `var r=0
if(true){
	r=1
}else{
	r=2
}
r`},
	{"if-chain", `def f(x){
	if(x<0){
		return -1
	}else{
		if(x==0){
			return 0
		}else{
			return 1
		}
	}
}
f(-5)+f(0)+f(5)`},
	{"while-form", `var i=0
var s=0
for(i<10){
	s=s+i
	i=i+1
}
s`},
	{"for-classic", `var s=0
for(var i=0;i<10;i=i+1){
	s=s+i
}
s`},
	{"nested-loop-break", `var i=0
var inner=0
for(;i<3;){
	i=i+1
	for(var j=0;j<5;j=j+1){
		inner=inner+1
		break
	}
}
inner`},
	{"loop-cond", `var i=0
var c=0
for(;i<100;){
	if(i%3==0){
		c=c+1
	}
	i=i+1
}
c`},
	{"recursion", `def fib(n){
	if(n<=1){
		return n
	}
	return fib(n-1)+fib(n-2)
}
fib(10)`},
	{"memo-fib", `def fib(x){
	if(x<=1){
		return x
	}else{
		var a=fib(x-1)
		var b=fib(x-2)
		return a+b
	}
}
fib(12)`},
	{"params", `def add(a,b){
	var z=a+b
	return z*2
}
add(3,4)`},
	{"array", `var a=[1,2,3,4]
var s=a[0]+a[3]
a[1]=9
s+a[1]`},
	{"map", `var m={1:10,2:20}
m[3]=30
m[1]+m[2]+m[3]`},
	{"globals", `var g=1
def bump(){
	return g+1
}
g=bump()
g+10`},
	{"string", `var s="hello"
var t=s+" world"
len(t)`},
	{"bool-logic", `var a=1
var b=0
var r=(a!=0 and b!=0) or (a>0)
r`},
	{"deep-expr", `var a=1
var b=2
var c=3
var d=(a+b)*(c-a)/(b+1)
d`},
	{"newton", `def sqrt(t){
	var i=10/2
	var const=0
	var tmp=9999
	for(;tmp>0.000000001;){
		i=i-(i**2-t)/(2*i)
		const=const+1
		if(i**2-t>0){
			tmp=i**2-t
		}else{
			tmp=-(i**2-t)
		}
	}
	return i
}
sqrt(2)`},
	{"nested-func-call", `def sq(x){
	return x*x
}
def quad(x){
	return sq(sq(x))
}
quad(2)`},
	{"early-return", `def f(x){
	if(x>0){
		return 1
	}
	return 0
}
f(5)+f(-5)`},
	{"shadow-blocks", `var x=1
if(true){
	var x=2
	x=x+10
}
x`},
	{"dead-branch", `var a=0
if(1==1){
	a=7
}else{
	a=9
}
a`},
	{"const-fold-branch", `var a=0
if(2*3>5){
	a=1
}
a`},
	{"compound", `var a=10
a+=5
a-=3
a*=2
a`},
}

// TestOptimizePreservesBehavior 是优化器最重要的防护：
// 对每个用例，未优化与优化后的执行结果必须完全一致。
func TestOptimizePreservesBehavior(t *testing.T) {
	for _, tc := range corpus {
		t.Run(tc.name, func(t *testing.T) {
			base := compileSrc(t, tc.src)
			want, wantErr := runBC(base)

			opt, err := ir.Optimize(base)
			if err != nil {
				if isUnsupported(err) {
					t.Skipf("optimizer declined this program: %v", err)
				}
				t.Fatalf("optimize failed: %v", err)
			}

			got, gotErr := runBC(opt)
			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("error behavior changed: base=%v optimized=%v", wantErr, gotErr)
			}
			if wantErr != nil {
				if wantErr.Error() != gotErr.Error() {
					t.Fatalf("error message changed:\n base=%v\n opt =%v", wantErr, gotErr)
				}
				return
			}
			if got != want {
				t.Fatalf("result changed: base=%s optimized=%s", want, got)
			}
		})
	}
}

// TestOptimizeNeverGrowsCode 守住"优化不会让代码变大"这条底线。
//
// 允许每个用例最多增长 3 字节：赋值表达式（如 g=bump()）在栈式字节码里
// 直接把值留在操作数栈上，而 IR 模型必须"存槽位 + 读回"，正好多一条
// 3 字节的 load/store。除此之外任何增长都视为回归。
func TestOptimizeNeverGrowsCode(t *testing.T) {
	const perCaseAllowance = 3
	totalBase, totalOpt := 0, 0
	for _, tc := range corpus {
		base := compileSrc(t, tc.src)
		opt, err := ir.Optimize(base)
		if err != nil {
			if isUnsupported(err) {
				t.Skipf("%s: optimizer declined: %v", tc.name, err)
			}
			t.Fatalf("%s: %v", tc.name, err)
		}
		growth := len(opt.Instruction) - len(base.Instruction)
		if growth > perCaseAllowance {
			t.Fatalf("%s: code grew by %d bytes (%d -> %d)",
				tc.name, growth, len(base.Instruction), len(opt.Instruction))
		}
		totalBase += len(base.Instruction)
		totalOpt += len(opt.Instruction)
	}
	if totalOpt >= totalBase {
		t.Fatalf("optimizer should shrink code overall: %d -> %d bytes", totalBase, totalOpt)
	}
	t.Logf("corpus total: %d -> %d bytes (-%d%%)",
		totalBase, totalOpt, 100*(totalBase-totalOpt)/totalBase)
}

func isUnsupported(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unsupported")
}

// TestOptimizeLowersInstructionCount 确认优化真的生效（不是原样返回）。
func TestOptimizeLowersInstructionCount(t *testing.T) {
	cases := []string{
		"1+2*3",
		"var a=0\nif(2*3>5){\n\ta=1\n}\na",
		"var a=1\na=a+1\na",
	}
	for _, src := range cases {
		base := compileSrc(t, src)
		opt, err := ir.Optimize(base)
		if err != nil {
			if isUnsupported(err) {
				t.Skipf("%q: %v", src, err)
			}
			t.Fatalf("%q: %v", src, err)
		}
		if len(opt.Instruction) >= len(base.Instruction) {
			t.Fatalf("%q: optimized code is not smaller: %d -> %d bytes",
				src, len(base.Instruction), len(opt.Instruction))
		}
	}
}

// 常量折叠必须与 VM 的实际语义一致（逐个运算符交叉验证）。
func TestConstantFoldingMatchesVM(t *testing.T) {
	cases := []string{
		"1+2", "1-2", "3*4", "7/2", "7%3", "2**10",
		"1.5+2", "1+2.5", "3.0*2", "7.0/2", "7.0%2", "2.0**3",
		`"a"+"b"`, `"a"+1`, `1+"a"`,
		"1<2", "1<=1", "2>1", "2>=3", "1==1", "1!=1",
		"1==1.0", "1.5<2", `"a"<"b"`, `"a"=="a"`,
		"true and false", "true or false", "not true", "not none",
		"-3", "-3.5", "+4",
		"none==none", "none==1", "true==false",
		"1/0", "0/0", "5%0",
	}
	for _, expr := range cases {
		t.Run(expr, func(t *testing.T) {
			base := compileSrc(t, expr)
			want, wantErr := runBC(base)
			opt, err := ir.Optimize(base)
			if err != nil {
				if isUnsupported(err) {
					t.Skipf("declined: %v", err)
				}
				t.Fatalf("optimize: %v", err)
			}
			got, gotErr := runBC(opt)
			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("error behavior changed: base=%v optimized=%v", wantErr, gotErr)
			}
			if wantErr == nil && got != want {
				t.Fatalf("%q: base=%s optimized=%s", expr, want, got)
			}
		})
	}
}

// 优化后的字节码必须能通过字节码自身的 CFG 校验（跳转目标都在范围内）。
func TestOptimizedBytecodeIsWellFormed(t *testing.T) {
	for _, tc := range corpus {
		base := compileSrc(t, tc.src)
		opt, err := ir.Optimize(base)
		if err != nil {
			continue
		}
		if _, err := cfg.Build(opt.Instruction); err != nil {
			t.Fatalf("%s: main instructions malformed: %v", tc.name, err)
		}
		for _, obj := range opt.Constants {
			fn, ok := obj.(object.CompiledFunc)
			if !ok {
				continue
			}
			if _, err := cfg.Build(fn.Instructions); err != nil {
				t.Fatalf("%s: function %s malformed: %v", tc.name, fn.FnName, err)
			}
		}
	}
}
