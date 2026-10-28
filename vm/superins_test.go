package vm_test

import (
	"strings"
	"testing"

	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/object"
	"Interpreter/vm"
)

// runBC 执行一次并返回最后一条语句的结果。
func runBC(bc *bytecode.Bytecode) (string, error) {
	m := vm.NewVM()
	if err := m.Run(bc); err != nil {
		return "", err
	}
	return m.LastPop().Inspect(), nil
}

// countOp 统计指令流里某个操作码出现的次数。
func countOp(ins code.Instructions, want code.Opcode) int {
	n := 0
	for i := 0; i < len(ins); {
		op := code.Opcode(ins[i])
		def, ok := code.Definitions[op]
		if !ok {
			return n
		}
		width := 1
		for _, w := range def.OperandWidth {
			width += w
		}
		if op == want {
			n++
		}
		i += width
	}
	return n
}

func countAll(bc *bytecode.Bytecode, want code.Opcode) int {
	n := countOp(bc.Instruction, want)
	for _, obj := range bc.Constants {
		if fn, ok := obj.(object.CompiledFunc); ok {
			n += countOp(fn.Instructions, want)
		}
	}
	return n
}

// 条件判断必须被融合成 OpCmpBranch（否则说明规则失效）。
func TestCmpBranchFires(t *testing.T) {
	cases := []string{
		"def fib(n){\n\tif(n<2){\n\t\treturn n\n\t}\n\treturn fib(n-1)+fib(n-2)\n}\nfib(5)",
		"var s=0\nfor(var i=0;i<10;i=i+1){\n\ts=s+i\n}\ns",
		"var a=1\nif(a>0){\n\ta=2\n}\na",
	}
	for _, src := range cases {
		bc := compileSrc(t, src)
		if n := countAll(bc, code.OpCmpBranch); n == 0 {
			t.Fatalf("%q: expected OpCmpBranch, bytecode:\n%s", src, bc.InsToString(bc.Instruction, 0, 0, bc.Symbols))
		}
	}
}

// 直接调用已知函数必须走 OpCallFuncConst（callee 不占操作数栈）。
func TestCallFuncConstFires(t *testing.T) {
	const src = "def sq(x){\n\treturn x*x\n}\nsq(5)"
	bc := compileSrc(t, src)
	if n := countAll(bc, code.OpCallFuncConst); n == 0 {
		t.Fatalf("expected OpCallFuncConst, bytecode:\n%s", bc.InsToString(bc.Instruction, 0, 0, bc.Symbols))
	}
	if got, err := runBC(bc); err != nil || got != "25" {
		t.Fatalf("result = %s, want 25", got)
	}
}

// 高阶调用（callee 从变量/参数来）必须继续用通用的 OpCallFunc。
func TestCallFuncStillUsedForDynamicCallee(t *testing.T) {
	const src = "def inc(x){\n\treturn x+1\n}\nvar g=inc\ng(3)"
	bc := compileSrc(t, src)
	if got, err := runBC(bc); err != nil || got != "4" {
		t.Fatalf("result = %s, err = %v, want 4", got, err)
	}
}

// OpCmpBranch 的三种操作数来源都要正确工作。
func TestCmpBranchOperandSources(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		// 局部 vs 常量
		{"def f(n){\n\tif(n<5){\n\t\treturn 1\n\t}\n\treturn 0\n}\nf(3)", "1"},
		{"def f(n){\n\tif(n<5){\n\t\treturn 1\n\t}\n\treturn 0\n}\nf(9)", "0"},
		// 全局 vs 常量
		{"var g=3\nif(g<5){\n\tg=1\n}else{\n\tg=0\n}\ng", "1"},
		// 局部 vs 局部
		{"def f(a,b){\n\tif(a<b){\n\t\treturn 1\n\t}\n\treturn 0\n}\nf(1,2)", "1"},
		// 字符串比较（走慢路径）
		{"def f(a){\n\tif(a<\"b\"){\n\t\treturn 1\n\t}\n\treturn 0\n}\nf(\"a\")", "1"},
		{"def f(a){\n\tif(a<\"b\"){\n\t\treturn 1\n\t}\n\treturn 0\n}\nf(\"c\")", "0"},
		// 相等 / 不等
		{"def f(a){\n\tif(a==3){\n\t\treturn 1\n\t}\n\treturn 0\n}\nf(3)", "1"},
		{"def f(a){\n\tif(a!=3){\n\t\treturn 1\n\t}\n\treturn 0\n}\nf(3)", "0"},
		// 浮点
		{"var x=1.5\nif(x<2.0){\n\tx=7\n}\nx", "7"},
	}
	for _, tc := range cases {
		bc := compileSrc(t, tc.src)
		got, err := runBC(bc)
		if err != nil {
			t.Fatalf("%q: %v", tc.src, err)
		}
		if got != tc.want {
			t.Fatalf("%q: result = %s, want %s", tc.src, got, tc.want)
		}
	}
}

// 条件里出现类型错误时，报错行号必须仍然指向写条件的那一行。
func TestCmpBranchErrorKeepsLine(t *testing.T) {
	src := "var a=1\n\nif(a<\"x\"){\n\ta=2\n}\na"
	bc := compileSrc(t, src)
	_, err := runBC(bc)
	if err == nil {
		t.Fatal("expected a runtime error comparing Int with String using <")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("error should point at line 3, got %v", err)
	}
}

// 参数个数不对时，错误必须指向调用点所在行。
func TestCallFuncConstArityErrorKeepsLine(t *testing.T) {
	src := "def f(a){\n\treturn a\n}\n\nf()"
	bc := compileSrc(t, src)
	_, err := runBC(bc)
	if err == nil {
		t.Fatal("expected a wrong-number-of-arguments error")
	}
	if !strings.Contains(err.Error(), "line 5") {
		t.Fatalf("error should point at line 5, got %v", err)
	}
	if !strings.Contains(err.Error(), "wrong number of arguments") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// 调用结果必须落回"callee 原本占的那个位置"：
// 在表达式中间调用时，后续运算要能拿到正确结果。
func TestCallFuncConstResultPlacement(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"def f(x){\n\treturn x+1\n}\n1+f(2)*3", "10"},
		{"def f(x){\n\treturn x\n}\nf(1)+f(2)+f(3)", "6"},
		{"def f(x){\n\treturn x*2\n}\nvar a=[f(1),f(2)]\na[0]+a[1]", "6"},
		{"def f(x){\n\treturn x\n}\nf(f(f(2)))", "2"},
	}
	for _, tc := range cases {
		bc := compileSrc(t, tc.src)
		got, err := runBC(bc)
		if err != nil {
			t.Fatalf("%q: %v", tc.src, err)
		}
		if got != tc.want {
			t.Fatalf("%q: result = %s, want %s", tc.src, got, tc.want)
		}
	}
}

// 深层递归 + OpCallFuncConst：帧必须正确展开、槽位不泄漏。
func TestCallFuncConstDeepRecursion(t *testing.T) {
	const src = `def down(n){
	if(n<=0){
		return 0
	}
	return down(n-1)+1
}
down(400)`
	bc := compileSrc(t, src)
	m := vm.NewVM()
	if err := m.Run(bc); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := m.LastPop().Inspect(); got != "400" {
		t.Fatalf("result = %s, want 400", got)
	}
	if depth := m.LocalsDepth(); depth != bc.MainLocals {
		t.Fatalf("locals leaked: %d, want %d", depth, bc.MainLocals)
	}
}

// 静态栈上界必须覆盖新的融合指令（否则会越界）。
func TestCmpBranchStaticStackBound(t *testing.T) {
	const src = `var s=0
for(var i=0;i<50;i=i+1){
	if(i<25){
		s=s+i
	}else{
		s=s-i
	}
}
s`
	bc := compileSrc(t, src)
	m := vm.NewVM()
	if err := m.Run(bc); err != nil {
		t.Fatalf("run: %v", err)
	}
	if peak := m.MaxStackDepth(); peak > bc.MainMaxStack {
		t.Fatalf("peak = %d exceeds static bound %d", peak, bc.MainMaxStack)
	}
}

// for(cond) 形式的循环（回边目标是 OpCmpBranch 本身）要能正确执行。
func TestCmpBranchWhileLoop(t *testing.T) {
	const src = `var i=5
var s=0
for(i>0){
	s=s+i
	i=i-1
}
s`
	bc := compileSrc(t, src)
	if got, err := runBC(bc); err != nil || got != "15" {
		t.Fatalf("result = %s, err = %v, want 15", got, err)
	}
}
