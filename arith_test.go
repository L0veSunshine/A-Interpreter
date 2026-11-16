package main

import (
	"fmt"
	"math"
	"strconv"
	"testing"
)

// ---------------------------------------------------------------------------
// 整数四则：逐值对照 Go 的语义
// ---------------------------------------------------------------------------

// intOpWant 给出 xlang 的整数运算结果（与 VM / IR 常量折叠共用同一套语义）：
//   - + - * 按 int 回绕；
//   - % 按截断取余（与 Go/C 一致，负数结果为负）；
//   - // 向负无穷取整（与 Python 一致），整数结果仍为整数；
//   - / 即使是整数也返回浮点，按 64 位精度展示。
func intOpWant(op string, a, b int) string {
	switch op {
	case "+":
		return strconv.Itoa(a + b)
	case "-":
		return strconv.Itoa(a - b)
	case "*":
		return strconv.Itoa(a * b)
	case "%":
		return strconv.Itoa(a % b)
	case "//":
		return strconv.Itoa(int(math.Floor(float64(a) / float64(b))))
	case "/":
		return strconv.FormatFloat(float64(a)/float64(b), 'f', -1, 64)
	}
	panic("unknown op " + op)
}

// 常量形式和"变量 + 运行时运算"形式都要覆盖：前者走 IR 常量折叠，
// 后者走 VM 的算术指令，两条路径的语义必须一致。
func TestIntegerArithmeticMatchesGo(t *testing.T) {
	for _, op := range []string{"+", "-", "*", "%", "/", "//"} {
		for a := -6; a <= 6; a++ {
			for b := -6; b <= 6; b++ {
				if (op == "%" || op == "/" || op == "//") && b == 0 {
					continue
				}
				want := intOpWant(op, a, b)

				literal := fmt.Sprintf("(%d) %s (%d)", a, op, b)
				if got := evalSrc(t, literal); got != want {
					t.Fatalf("%s => want %s, got %s", literal, want, got)
				}

				viaVars := fmt.Sprintf("var a=%d\nvar b=%d\na %s b", a, b, op)
				if got := evalSrc(t, viaVars); got != want {
					t.Fatalf("%s => want %s, got %s", viaVars, want, got)
				}
			}
		}
	}
}

func TestIntegerDivisionByZero(t *testing.T) {
	for _, src := range []string{"1/0", "1%0", "-1/0", "0%0", "1//0", "-7//0"} {
		expectErr(t, src, "division by zero")
	}
}

// 整数 // 向负无穷取整，结果类型保持 Int；浮点 // 结果为 Float。
func TestFloorDivision(t *testing.T) {
	cases := []struct{ src, want string }{
		{"7//2", "3"},
		{"-7//2", "-4"},
		{"7//-2", "-4"},
		{"-7//-2", "3"},
		{"-6//2", "-3"},
		{"0//5", "0"},
		{"7//2*2", "6"},
		{"2*7//2", "7"},
		{"7.0//2", "3"},
		{"-7.5//2", "-4"},
		{"7//2.5", "2"},
		{"type(7//2)", "<class 'Int'>"},
		{"type(7.0//2)", "<class 'Float'>"},
		{"var a=-7\na//=2\na", "-4"},
		{"var a=7.5\na//=2\na", "3"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}
}

func TestFloatDivisionAndModuloByZero(t *testing.T) {
	// 浮点除零与整数一样报错（不是 IEEE 的 Inf/NaN）；
	// 浮点取模同样要报错，而不是静默返回 NaN。
	for _, src := range []string{"1.0/0.0", "1/0.0", "1.0%0.0", "0.0/0.0", "1.0//0.0"} {
		expectErr(t, src, "division by zero")
	}
}

func TestIntegerOverflowWraps(t *testing.T) {
	if strconv.IntSize != 64 {
		t.Skip("回绕结果依赖 64 位 int")
	}
	var maxInt int = math.MaxInt64
	var minInt int = math.MinInt64
	// 语言里没有字面量能直接表示 MinInt64（9223372036854775808 会溢出），
	// 这里用 (-maxInt-1) 构造。
	minSrc := fmt.Sprintf("(-%d-1)", maxInt)
	cases := []struct{ src, want string }{
		{fmt.Sprintf("%d+1", maxInt), strconv.Itoa(maxInt + 1)},
		{minSrc + "-1", strconv.Itoa(minInt - 1)},
		{fmt.Sprintf("%d*2", maxInt), strconv.Itoa(maxInt * 2)},
	}
	for _, tc := range cases {
		if got := evalSrc(t, tc.src); got != tc.want {
			t.Fatalf("%s => want %s, got %s", tc.src, tc.want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 浮点
// ---------------------------------------------------------------------------

func TestFloatArithmetic(t *testing.T) {
	sqrt2 := strconv.FormatFloat(math.Sqrt2, 'f', -1, 64)
	cases := []struct{ src, want string }{
		{"1/3", strconv.FormatFloat(1.0/3.0, 'f', -1, 64)},
		{"0.1+0.2", "0.30000000000000004"},
		{"2**0.5", sqrt2},
		{"1.5+1", "2.5"},
		{"1+1.5", "2.5"},
		{"7/2", "3.5"},
		{"10/5", "2"},
		{"1e3", "1000"},
		{"5.0%2.0", "1"},
		{"0.0/5.0", "0"},
		{"2**-1", "0.5"},
		{"2.0**3", "8"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}
}

// 展示精度是 64 位：1/3 不能被截断成 float32 的 0.33333334。
func TestFloatInspectKeepsDoublePrecision(t *testing.T) {
	expect(t, "1/3", "0.3333333333333333")
	expect(t, "2**0.5", "1.4142135623730951")
}

func TestDivisionAndPowerKeepDocumentedResultTypes(t *testing.T) {
	cases := []struct{ src, want string }{
		{"type(1+2)", "<class 'Int'>"},
		{"type(1.0+2)", "<class 'Float'>"},
		{"type(7%3)", "<class 'Int'>"},
		{"type(7.0%3)", "<class 'Float'>"},
		{"type(1/2)", "<class 'Float'>"},
		{"type(4/2)", "<class 'Float'>"},
		{"type(2**2)", "<class 'Float'>"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}
}

func TestIntConversions(t *testing.T) {
	cases := []struct{ src, want string }{
		{"int(7/2)", "3"},
		{"int(-7/2)", "-3"},
		{"int(true)", "1"},
		{"int(false)", "0"},
		{`int("42")`, "42"},
		{"float(3)", "3"},
		{"float(true)", "1"},
		{"float(false)", "0"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}
}

// ---------------------------------------------------------------------------
// 比较
// ---------------------------------------------------------------------------

// 2^53 以上 float64 无法区分相邻整数，比较必须走精确路径（VM 与常量折叠都要）。
func TestIntegerComparisonIsExact(t *testing.T) {
	cases := []struct{ src, want string }{
		{"9007199254740993 == 9007199254740992", "false"},
		{"9007199254740993 != 9007199254740992", "true"},
		{"9007199254740993 > 9007199254740992", "true"},
		{"9007199254740992 < 9007199254740993", "true"},
		{"9223372036854775807 == 9223372036854775806", "false"},
		{"9223372036854775807 > 9223372036854775806", "true"},
		{"(-9223372036854775807-1) < -9223372036854775807", "true"},
		{"2**63 > 9223372036854775807", "true"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}

	// 变量形式：不经过常量折叠
	viaVars := `var a=9007199254740993
var b=9007199254740992
[a == b, a > b]`
	expect(t, viaVars, "[false, true]")
}

func TestIntFloatComparisonIsExact(t *testing.T) {
	cases := []struct{ src, want string }{
		{"9007199254740993 == 9007199254740992.0", "false"},
		{"9007199254740993 > 9007199254740992.0", "true"},
		{"9007199254740992 == 9007199254740992.0", "true"},
		{"1 == 1.0", "true"},
		{"1 < 1.5", "true"},
		{"2 > 1.5", "true"},
		{"-1.5 < -1", "true"},
		{"-1.5 > -2", "true"},
		{"0.1+0.2 == 0.3", "false"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}
}

func TestRandomIntegerComparisonMatchesGo(t *testing.T) {
	values := []int{
		0, 1, -1, 2, -7,
		1<<53 - 1, 1 << 53, 1<<53 + 1,
		1 << 62, -(1 << 62),
		math.MaxInt64,
	}
	ops := []struct {
		token string
		fn    func(a, b int) bool
	}{
		{"==", func(a, b int) bool { return a == b }},
		{"!=", func(a, b int) bool { return a != b }},
		{"<", func(a, b int) bool { return a < b }},
		{"<=", func(a, b int) bool { return a <= b }},
		{">", func(a, b int) bool { return a > b }},
		{">=", func(a, b int) bool { return a >= b }},
	}
	for _, a := range values {
		for _, b := range values {
			for _, op := range ops {
				src := fmt.Sprintf("%d %s %d", a, op.token, b)
				want := strconv.FormatBool(op.fn(a, b))
				if got := evalSrc(t, src); got != want {
					t.Fatalf("%s => want %s, got %s", src, want, got)
				}
			}
		}
	}
}

func TestNaNComparisonIsUnordered(t *testing.T) {
	nan := `float("nan")`
	cases := []struct{ src, want string }{
		{nan + " == " + nan, "false"},
		{nan + " != " + nan, "true"},
		{nan + " < 1", "false"},
		{nan + " <= 1", "false"},
		{nan + " > 1", "false"},
		{nan + " >= 1", "false"},
		{`1 == float("nan")`, "false"},
		{`1 != float("nan")`, "true"},
		{`1 < float("nan")`, "false"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}
}

func TestStringAndCompositeComparison(t *testing.T) {
	cases := []struct{ src, want string }{
		{`"abc" < "abd"`, "true"},
		{`"abd" <= "abc"`, "false"},
		{`"abc" == "abc"`, "true"},
		{`"Z" < "a"`, "true"},
		{`1 == "1"`, "false"},
		{`"1" == 1`, "false"},
		{`[1,2] == [1,2]`, "true"},
		{`[1,2] == [1,3]`, "false"},
		{`[1,[2]] == [1,[2]]`, "true"},
		{`{1:2} == {1:2}`, "true"},
		{`{1:2} == {1:3}`, "false"},
		{"none == none", "true"},
		{"none != 0", "true"},
		{"true == true", "true"},
		{"true == 1", "false"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}

	for _, src := range []string{`1 < "a"`, "true < 1", `none < none`, `[1] < [2]`} {
		expectErr(t, src, "unsupport")
	}
}

// ---------------------------------------------------------------------------
// 真值 / not / and / or
// ---------------------------------------------------------------------------

// not 必须走与 if / and / or 相同的真值判定（曾经 not 0 会直接运行时错误）。
func TestNotUsesTruthiness(t *testing.T) {
	cases := []struct {
		expr  string
		truth bool
	}{
		{"0", false}, {"1", true}, {"-1", true},
		{"0.0", false}, {"0.5", true},
		{`""`, false}, {`"x"`, true},
		{"none", false}, {"true", true}, {"false", false},
		{"[0]", true}, {"[]", true}, // 容器不看长度，始终为真
		{"{}", true},
	}
	for _, tc := range cases {
		notWant := strconv.FormatBool(!tc.truth)
		if got := evalSrc(t, "not "+tc.expr); got != notWant {
			t.Fatalf("not %s => want %s, got %s", tc.expr, notWant, got)
		}
		ifWant := "0"
		if tc.truth {
			ifWant = "1"
		}
		src := fmt.Sprintf("if(%s){1}else{0}", tc.expr)
		if got := evalSrc(t, src); got != ifWant {
			t.Fatalf("%s => want %s, got %s", src, ifWant, got)
		}
	}
}

func TestLogicalOperatorsReturnBooleansAndShortCircuit(t *testing.T) {
	cases := []struct{ src, want string }{
		{"1 and 2", "true"},
		{"0 and 3", "false"},
		{"0 or 3", "true"},
		{`"" or "x"`, "true"},
		{"true or false and false", "true"}, // and 优先级高于 or
		{"not false and true", "true"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}

	src := `var hits=0
def bump(){
	hits+=1
	return true
}
var a=false and bump()
var b=true or bump()
[a, b, hits]`
	expect(t, src, "[false, true, 0]")
}

// ---------------------------------------------------------------------------
// 复合赋值与优先级
// ---------------------------------------------------------------------------

func TestCompoundAssignment(t *testing.T) {
	cases := []struct{ src, want string }{
		{"var x=1\nx+=2\nx", "3"},
		{"var x=7\nx-=2\nx", "5"},
		{"var x=3\nx*=4\nx", "12"},
		{"var x=7\nx/=2\nx", "3.5"},
		{"var x=7\nx%=4\nx", "3"},
		{"var x=-7\nx%=4\nx", "-3"}, // 截断取余，符号跟被除数
		{"var x=3\nx**=2\nx", "9"},
		{`var s="a"` + "\ns+=\"b\"\ns", "ab"},
		{"def f(){\n\tvar x=10\n\tx-=4\n\treturn x\n}\nf()", "6"},
		{"var g=5\ndef f(){\n\tg+=1\n}\nf()\ng", "6"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}
}

func TestOperatorPrecedence(t *testing.T) {
	cases := []struct{ src, want string }{
		{"2+3*4", "14"},
		{"(2+3)*4", "20"},
		{"2*3+4*5", "26"},
		{"10-4-3", "3"},
		{"2**3*2", "16"},
		{"2*3**2", "18"},
		{"2**-2", "0.25"},
		{"not 1==2", "true"},
		{"1<2 and 3>2", "true"},
		// 已知的两处与 Python 不同：** 左结合、一元负号比 ** 更紧。
		// 见 README「语义要点」；若要改成 Python 语义，这两条断言需要同步修改。
		{"2**3**2", "64"},
		{"-2**2", "4"},
	}
	for _, tc := range cases {
		expect(t, tc.src, tc.want)
	}
}

// ---------------------------------------------------------------------------
// 错误定位与类型错误
// ---------------------------------------------------------------------------

func TestArithmeticErrorCarriesLine(t *testing.T) {
	src := `var a=1
var b=0
var c=a/b`
	expectErr(t, src, "line 3")
}

func TestArithmeticTypeErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"1+true", "unsupported types for binary operation"},
		{"none+1", "unsupported types for binary operation"},
		{"[1]+[2]", "unsupported types for binary operation"},
		{`"a"-1`, "unknown operator"},
		{`-"a"`, "unsupported type for negation"},
	}
	for _, tc := range cases {
		expectErr(t, tc.src, tc.want)
	}
}

// ---------------------------------------------------------------------------
// 随机表达式差分测试：同一棵表达式树在 Go 与 xlang 里求值必须相等
// ---------------------------------------------------------------------------

type intExpr struct {
	op          byte // 0 表示叶子
	left, right *intExpr
	value       int
}

func (e *intExpr) eval() int {
	switch e.op {
	case '+':
		return e.left.eval() + e.right.eval()
	case '-':
		return e.left.eval() - e.right.eval()
	case '*':
		return e.left.eval() * e.right.eval()
	}
	return e.value
}

func (e *intExpr) render() string {
	if e.op == 0 {
		return strconv.Itoa(e.value)
	}
	return fmt.Sprintf("(%s %c %s)", e.left.render(), e.op, e.right.render())
}

func TestRandomArithmeticMatchesGo(t *testing.T) {
	// 固定种子的 LCG，失败时用例可复现
	state := uint32(20240517)
	next := func(n int) int {
		state = state*1664525 + 1013904223
		return int(state>>8) % n
	}
	var build func(depth int) *intExpr
	build = func(depth int) *intExpr {
		if depth == 0 || next(10) < 3 {
			return &intExpr{value: next(41) - 20}
		}
		op := "+-*"[next(3)]
		return &intExpr{op: op, left: build(depth - 1), right: build(depth - 1)}
	}

	const cases = 150
	checked := 0
	for attempt := 0; checked < cases && attempt < cases*20; attempt++ {
		e := build(3)
		src := e.render()
		want := strconv.Itoa(e.eval())
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%s => want %s, got %s", src, want, got)
		}
		checked++
	}
	if checked != cases {
		t.Fatalf("only generated %d cases", checked)
	}
}
