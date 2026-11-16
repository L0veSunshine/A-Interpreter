package main

import (
	"testing"

	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/parser"
	vm2 "Interpreter/vm"
)

// evalWithOptimizer 编译并执行，opt 控制是否走 IR/SSA 优化管线。
func evalWithOptimizer(t *testing.T, src string, optimize bool) string {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(src))
	node := p.Parse()
	if p.HasError() {
		t.Fatalf("parse error: %v", p.Errs())
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.SetOptimize(optimize)
	c.Compile(node)
	if c.HasError() {
		t.Fatalf("compile error: %v", c.Errs())
	}
	m := vm2.NewVM()
	if err := m.Run(c.ByteCode()); err != nil {
		t.Fatalf("runtime error（optimize=%v）: %v", optimize, err)
	}
	return m.LastPop().Inspect()
}

// expectSameWithAndWithoutOptimizer 断言"优化开/关结果必须一致"。
//
// 优化器的契约是行为等价，所以这是最强的黑盒检查：只要有任何一处
// SSA / 槽位分配 / phi 拷贝出错，两条路径的结果就会不同。
func expectSameWithAndWithoutOptimizer(t *testing.T, name, src, want string) {
	t.Helper()
	plain := evalWithOptimizer(t, src, false)
	if plain != want {
		t.Fatalf("%s: 未优化结果就不对：want %s, got %s", name, want, plain)
	}
	if opt := evalWithOptimizer(t, src, true); opt != want {
		t.Fatalf("%s: 优化后结果不对（-O 结果 %s，未优化 %s，期望 %s）", name, opt, plain, want)
	}
}

// ---------------------------------------------------------------------------
// 回归：循环携带变量（phi）的三类错误
// ---------------------------------------------------------------------------

// 累加器 + 循环变量：两个 phi 在循环头同时活跃。
// 曾经因为 phi 拷贝"读-写-读-写"逐条执行，`s + i` 读到的是已经自增的 i，
// 于是 g(5) 从 10 变成 15（多算一轮）。
func TestLoopAccumulatorWithCarriedVariable(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"for-init", `def g(n){
	var s = 0
	for(var i = 0; i < n; i += 1){
		s += i
	}
	return s
}
g(5)`, "10"},
		{"while-form", `def g(n){
	var s = 0
	var i = 0
	for(; i < n; i += 1){
		s += i
	}
	return s
}
g(5)`, "10"},
		{"two-carried", `def g(n){
	var s = 0
	var k = 1
	for(var i = 0; i < n; i += 1){
		s += i
		k += 2
	}
	return [s, k]
}
g(5)`, "[10, 11]"},
		{"nested-loops", `def g(n){
	var t = 0
	for(var i = 0; i < n; i += 1){
		for(var j = 0; j < n; j += 1){
			t += i * j
		}
	}
	return t
}
g(4)`, "36"},
	}
	for _, tc := range cases {
		expectSameWithAndWithoutOptimizer(t, tc.name, tc.src, tc.want)
	}
}

// 交换两个数组元素：`t = a[i]` 的值不能被"就地展开"到第二次写之后，
// 否则读到的是已经被覆盖的 a[i]（实测会破坏快排分区）。
func TestSwapThroughArrayTemp(t *testing.T) {
	src := `def f(a, i, j){
	var t = a[i]
	a[i] = a[j]
	a[j] = t
	return 0
}
var arr = [3, 1, 2]
f(arr, 0, 1)
arr`
	expectSameWithAndWithoutOptimizer(t, "swap", src, "[1, 3, 2]")
}

// 递归调用夹在两次数组写之间：调用的求值不能被推迟到后面的写之后，
// 否则被调方看到的是已经回填过的数组（n-queens 因此退化成暴力枚举）。
func TestCallBetweenArrayWrites(t *testing.T) {
	src := `def nq(n, row, cols){
	if(row == n){
		return 1
	}
	var total = 0
	for(var c = 0; c < n; c += 1){
		if(cols[c] == 0){
			cols[c] = 1
			total += nq(n, row + 1, cols)
			cols[c] = 0
		}
	}
	return total
}
nq(4, 0, [0, 0, 0, 0])`
	// 只约束"每列一个"，所以 n=4 的放法是 4! = 24 种
	expectSameWithAndWithoutOptimizer(t, "nqueens-4", src, "24")
}

// 数组写之后读：值必须在使用点反映最新的内存状态。
func TestReadAfterArrayWrite(t *testing.T) {
	src := `def f(a){
	a[0] = 7
	return a[0] + len(a)
}
var arr = [1, 2]
f(arr)`
	expectSameWithAndWithoutOptimizer(t, "read-after-write", src, "9")
}

// 循环变量在循环体里被原地修改的容器：实参与 phi 目标共用槽位时，
// 边上的拷贝就是恒等拷贝，可以整条省掉（coalescing，纯收益）。
func TestLoopCarriedContainerStaysCorrect(t *testing.T) {
	src := `def fill(n){
	var a = [0, 0, 0, 0]
	for(var i = 0; i < n; i += 1){
		a[i] = i * i
	}
	return a
}
fill(4)`
	expectSameWithAndWithoutOptimizer(t, "loop-carried-array", src, "[0, 1, 4, 9]")
}
