package vm_test

import (
	"strings"
	"testing"

	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/object"
	"Interpreter/vm"
)

// MaxStack 一旦算小，VM 就会越界（现在靠 recover 兜住，但那是最后防线）。
// 所以必须验证：静态上界足够用。
//
// 两层验证：
//  1. 不含函数调用的程序，真实峰值必须 ≤ main 的上界（严格比对）；
//  2. 所有程序都必须正常执行，且不能出现 "internal: operand stack overflow"
//     —— 那说明某处上界算小了（递归程序的峰值随调用深度增长，
//     所以只对第 1 类做数值比对）。
func TestMaxStackIsRealUpperBound(t *testing.T) {
	cases := []string{
		"1+2*3",
		"9/2-9*8.102*2",
		"var a=1\na=a+1\na",
		"var a=0\nif(2*3>5){\n\ta=1\n}else{\n\ta=2\n}\na",
		"var s=0\nfor(var i=0;i<10;i=i+1){\n\ts=s+i*2\n}\ns",
		"var s=0\nfor(var i=0;i<5;i=i+1){\n\tfor(var j=0;j<5;j=j+1){\n\t\ts=s+j\n\t}\n}\ns",
		"var a=[1,2,3]\nvar s=0\nfor(var i=0;i<3;i=i+1){\n\ts=s+a[i]\n}\ns",
		"var m={1:10,2:20}\nm[3]=30\nm[1]+m[2]+m[3]",
		"def f(x){\n\treturn x*2+1\n}\nf(3)+f(4)",
		"def fib(n){\n\tif(n<2){\n\t\treturn n\n\t}\n\treturn fib(n-1)+fib(n-2)\n}\nfib(12)",
		"len([1,2,3])+len(\"abcd\")",
		"var x=1\nif(x>0){\n\tx=2\n\tif(x>1){\n\t\tx=3\n\t}\n}\nx",
		`var s="a"
for(var i=0;i<5;i=i+1){
	s=s+"b"
}
len(s)`,
	}
	for _, src := range cases {
		bc := compileSrc(t, src)

		// main 与每个函数的上界都必须等于精确分析的结果
		want, err := code.MaxStack(bc.Instruction)
		if err != nil {
			t.Fatalf("%q: MaxStack: %v", src, err)
		}
		if bc.MainMaxStack != want {
			t.Fatalf("%q: MainMaxStack = %d, want %d", src, bc.MainMaxStack, want)
		}
		hasFunc := false
		for _, obj := range bc.Constants {
			fn, ok := obj.(object.CompiledFunc)
			if !ok {
				continue
			}
			hasFunc = true
			fwant, ferr := code.MaxStack(fn.Instructions)
			if ferr != nil {
				t.Fatalf("%q: %s: MaxStack: %v", src, fn.FnName, ferr)
			}
			if fn.MaxStack != fwant {
				t.Fatalf("%q: %s: MaxStack = %d, want %d", src, fn.FnName, fn.MaxStack, fwant)
			}
		}

		m := vm.NewVM()
		if err := m.Run(bc); err != nil {
			t.Fatalf("%q: run: %v", src, err)
		}
		// 不含函数调用时，全局峰值就是 main 的相对深度，可以严格比对
		if !hasFunc {
			if peak := m.MaxStackDepth(); peak > bc.MainMaxStack {
				t.Fatalf("%q: peak = %d exceeds static bound %d", src, peak, bc.MainMaxStack)
			}
		}
	}
}

// 递归 + 深表达式：静态上界小、但执行很多轮，验证不会因为"没检查"而越界。
func TestMaxStackDeepExpression(t *testing.T) {
	// 构造一个很深的左结合表达式
	src := "1"
	for i := 0; i < 60; i++ {
		src += "+1"
	}
	bc := compileSrc(t, src)
	m := vm.NewVM()
	if err := m.Run(bc); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := m.LastPop().Inspect(); got != "61" {
		t.Fatalf("result = %s, want 61", got)
	}
	if peak := m.MaxStackDepth(); peak > bc.MainMaxStack {
		t.Fatalf("peak = %d exceeds static bound %d", peak, bc.MainMaxStack)
	}
}

// 一个字节码被反复执行（循环）时，峰值不应随迭代次数增长。
func TestMaxStackStableAcrossIterations(t *testing.T) {
	src := `var s=0
for(var i=0;i<200;i=i+1){
	s=s+i*2-1
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

// 安全网：万一静态上界被算小（或字节码是手工拼的），执行必须返回错误，
// 而不是让进程 panic。这确保"去掉每指令检查"不会把 bug 变成崩溃。
func TestStackOverflowBecomesError(t *testing.T) {
	var prog code.Instructions
	// 故意压入远超上界声明的值个数
	for i := 0; i < InitStackForTest()*2; i++ {
		prog = append(prog, code.Make(code.OpNull)...)
	}
	bc := &bytecode.Bytecode{
		Instruction: prog,
		Constants:   []object.Object{object.Int{Value: 1}},
		// 谎报上界：只留 1 个槽位
		MainMaxStack: 1,
	}
	m := vm.NewVM()
	err := m.Run(bc)
	if err == nil {
		t.Fatal("expected an error when the static stack bound is too small")
	}
	if !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("error should mention overflow, got %v", err)
	}
}

// InitStackForTest 暴露初始操作数栈大小，供上面的用例构造溢出场景。
func InitStackForTest() int { return vm.InitStackSize }
