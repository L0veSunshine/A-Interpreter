package main

import (
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/parser"
	vm2 "Interpreter/vm"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// runErr 编译并执行一段程序，返回运行期错误（解析/编译错误通过 t.Fatalf 暴露）。
func runErr(t *testing.T, src string) error {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(src))
	node := p.Parse()
	if p.HasError() {
		t.Fatalf("parse error: %v", p.Errs())
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.Compile(node)
	if c.HasError() {
		t.Fatalf("compile error: %v", c.Errs())
	}
	vm := vm2.NewVM()
	return vm.Run(c.ByteCode())
}

// ---------------------------------------------------------------------------
// 栈溢出 / 栈高度
// ---------------------------------------------------------------------------

// evalMaxDepth 返回程序执行期间操作数栈的最大高度。
func evalMaxDepth(t *testing.T, src string) int {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(src))
	node := p.Parse()
	if p.HasError() {
		t.Fatalf("parse error: %v", p.Errs())
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.Compile(node)
	if c.HasError() {
		t.Fatalf("compile error: %v", c.Errs())
	}
	vm := vm2.NewVM()
	if err := vm.Run(c.ByteCode()); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	return vm.MaxStackDepth()
}

// 语句级表达式不弹栈的话，循环里每轮泄漏一个槽位，几千轮后就会 stack overflow。
func TestNoStackLeakForCallStatementInLoop(t *testing.T) {
	src := `def g(){
	return 1
}
var i=0
for(;i<5000;){
	i=i+1
	g()
}
i`
	if got := evalSrc(t, src); got != "5000" {
		t.Fatalf("want 5000, got %s", got)
	}
	if depth := evalMaxDepth(t, src); depth > 32 {
		t.Fatalf("stack should not grow per iteration, max depth = %d", depth)
	}
}

func TestNoStackLeakForIfStatementInLoop(t *testing.T) {
	src := `var i=0
var s=0
for(;i<5000;){
	i=i+1
	if(i>0){
		s=s+1
	}
}
s`
	if got := evalSrc(t, src); got != "5000" {
		t.Fatalf("want 5000, got %s", got)
	}
	if depth := evalMaxDepth(t, src); depth > 32 {
		t.Fatalf("stack should not grow per iteration, max depth = %d", depth)
	}
}

// if 的两个分支必须留下相同的栈高度，否则汇合点之后的取值会错位。
func TestIfElseKeepsStackBalanced(t *testing.T) {
	src := `var r=0
if (true){
	var q=1
	r=r+q
}else{
	var q=2
	r=r+q
}
r`
	if got := evalSrc(t, src); got != "1" {
		t.Fatalf("want 1, got %s", got)
	}
}

// 分支里的赋值不能被跨基本块的 peephole 融合掉。
func TestPeepholeNotAppliedAcrossBranches(t *testing.T) {
	src := `var a=0
var b=0
if (a==0){
	a=5
}else{
	a=7
}
a`
	if got := evalSrc(t, src); got != "5" {
		t.Fatalf("want 5, got %s", got)
	}
}

// 同一符号连续两次取值不能被第二次也省掉（曾经让 a=a+a 少压一个值）。
func TestPeepholeAppliesOnlyOncePerStore(t *testing.T) {
	cases := map[string]string{
		"var a=1\na=a+a\na":   "2",
		"var a=1\na=a*2\na":   "2",
		"var a=1\na=a+a+1\na": "3",
	}
	for src, want := range cases {
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%q: want %s, got %s", src, want, got)
		}
	}
}

// 深递归必须返回错误，而不是 panic（索引越界 / 栈越界）。
func TestRunawayRecursionReturnsError(t *testing.T) {
	src := `def f(n){
	if(n<=0){
		return 0
	}
	return 1+f(n-1)
}
f(200000)`
	err := runErr(t, src)
	if err == nil {
		t.Fatal("runaway recursion should fail with an overflow error")
	}
	if !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("error should mention overflow, got %v", err)
	}
	if !strings.Contains(err.Error(), "f") {
		t.Fatalf("error should mention the function name, got %v", err)
	}
}

// 操作数栈按需增长后，几千层的深递归应当正常跑通。
func TestDeepRecursionWorks(t *testing.T) {
	src := `def f(n){
	if(n<=0){
		return 0
	}
	return 1+f(n-1)
}
f(5000)`
	if got := evalSrc(t, src); got != "5000" {
		t.Fatalf("want 5000, got %s", got)
	}
}

// 适度的递归仍然要能正常工作。
func TestRecursionWithinLimit(t *testing.T) {
	src := `def f(n){
	if(n<=0){
		return 0
	}
	return 1+f(n-1)
}
f(200)`
	if got := evalSrc(t, src); got != "200" {
		t.Fatalf("want 200, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// LastPop 语义
// ---------------------------------------------------------------------------

func TestLastPopReturnsLastStatementValue(t *testing.T) {
	cases := map[string]string{
		"42":                        "42",
		"1+2*3":                     "7",
		"var a=1\na":                "1",
		"var a=1\na=a+1\na":         "2",
		"var a=1\nvar b=2\nb":       "2",
		"var a=1\nif(a==1){a=3}\na": "3",
		"var a=1\nif(a==1){a=3}":    "None",
		"def f(){return 5}\nf()":    "5",
		"var a=[1,2]\na.append(3)":  "None",
	}
	for src, want := range cases {
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%q: want %s, got %s", src, want, got)
		}
	}
}

// 程序没有表达式语句时，LastPop 返回 None 而不是崩。
func TestLastPopWithoutExpressionStatement(t *testing.T) {
	if got := evalSrc(t, "var a=1"); got != "None" {
		t.Fatalf("want None, got %s", got)
	}
	if got := evalSrc(t, ""); got != "None" {
		t.Fatalf("empty program: want None, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// 值语义：相等、真值
// ---------------------------------------------------------------------------

func TestStructuralEquality(t *testing.T) {
	cases := map[string]string{
		"[1,2]==[3,4]": "false",
		"[1,2]==[1,2]": "true",
		"[1,2]!=[1,2]": "false",
		`"a"=="a"`:     "true",
		"1==1.0":       "true",
		"none==none":   "true",
	}
	for src, want := range cases {
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%q: want %s, got %s", src, want, got)
		}
	}
}

func TestLogicOperatorsUseTruthiness(t *testing.T) {
	cases := map[string]string{
		"true and false": "false",
		"true or false":  "true",
		"1 and 2":        "true",
		"0 or 0":         "false",
		"not true":       "false",
		"!true":          "false",
		"!false":         "true",
	}
	for src, want := range cases {
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%q: want %s, got %s", src, want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 运算优先级 / 短路求值
// ---------------------------------------------------------------------------

// and/or 的优先级必须低于比较运算，否则 a!=0 and 10/a>1 会先算 a and 10。
func TestAndOrPrecedence(t *testing.T) {
	cases := map[string]string{
		"true or false and false": "true",  // or < and
		"false and false or true": "true",  // (false and false) or true
		"1+2*3":                   "7",     // * 高于 +
		"2**3*2":                  "16",    // ** 高于 *
		"1+2==3":                  "true",  // + 高于 ==
		"not 1==2":                "true",  // not 作用于整个比较
		"not true and true":       "false", // (not true) and true
	}
	for src, want := range cases {
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%q: want %s, got %s", src, want, got)
		}
	}
}

// and 必须短路：左边为假时右边不能被求值。
func TestAndShortCircuits(t *testing.T) {
	src := `var a=0
var r=a!=0 and 10/a>1
r`
	if got := evalSrc(t, src); got != "false" {
		t.Fatalf("want false, got %s", got)
	}
	src2 := `var a=2
var r=a!=0 and 10/a>1
r`
	if got := evalSrc(t, src2); got != "true" {
		t.Fatalf("want true, got %s", got)
	}
}

// or 必须短路：左边为真时右边不能被求值。
func TestOrShortCircuits(t *testing.T) {
	src := `var a=0
var r=a==0 or 10/a>1
r`
	if got := evalSrc(t, src); got != "true" {
		t.Fatalf("want true, got %s", got)
	}
}

// 短路还应该避免右侧的副作用。
func TestShortCircuitSkipsSideEffects(t *testing.T) {
	src := `var n=0
def bump(){
	n=n+1
	return true
}
var r=false and bump()
n`
	if got := evalSrc(t, src); got != "0" {
		t.Fatalf("and should not evaluate bump, n = %s", got)
	}
	src2 := `var n=0
def bump(){
	n=n+1
	return true
}
var r=true or bump()
n`
	if got := evalSrc(t, src2); got != "0" {
		t.Fatalf("or should not evaluate bump, n = %s", got)
	}
}

// ---------------------------------------------------------------------------
// 函数 / 递归 / 记忆化
// ---------------------------------------------------------------------------

func TestMemoizedFib(t *testing.T) {
	src := `var cache={}
def fib(x){
	if(x<=1){
		return x
	}else{
		if(cache[x]!=none){
			return cache[x]
		}else{
			var res=fib(x-1)+fib(x-2)
			cache[x]=res
			return res
		}
	}
}
fib(15)`
	if got := evalSrc(t, src); got != "610" {
		t.Fatalf("want 610, got %s", got)
	}
}

// 局部变量槽位不能被写到别的槽位上（曾经因为 replaceLast 清空操作数而写错槽）。
func TestLocalSlotOperandsArePreserved(t *testing.T) {
	src := `def f(x){
	if(x>0){
		var res=x*10
		var other=x*100
		cache=res+other
	}
	return cache
}
var cache=0
f(3)`
	if got := evalSrc(t, src); got != "330" {
		t.Fatalf("want 330, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// Map
// ---------------------------------------------------------------------------

func TestMapInspectIsDeterministic(t *testing.T) {
	src := `var m={}
m[3]=30
m[1]=10
m[2]=20
m["k"]=1
m`
	first := evalSrc(t, src)
	for i := 0; i < 5; i++ {
		if got := evalSrc(t, src); got != first {
			t.Fatalf("map output should be stable: %q vs %q", first, got)
		}
	}
	for _, part := range []string{"1: 10", "2: 20", "3: 30", `"k": 1`} {
		if !strings.Contains(first, part) {
			t.Fatalf("%q should contain %q", first, part)
		}
	}
}

func TestMapLenMatchesStoreSize(t *testing.T) {
	src := `var m={1:2,3:4}
m[5]=6
len(m)`
	if got := evalSrc(t, src); got != "3" {
		t.Fatalf("want 3, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// 错误而不是 panic
// ---------------------------------------------------------------------------

func TestRuntimeErrorsInsteadOfPanic(t *testing.T) {
	cases := map[string]string{
		"1/0":                            "division by zero",
		"var a=[1]\nvar c=a[5]":          "out of range",
		"var a=1\na()":                   "calling non-function",
		"var a=[1]\na.nosuch()":          "don't has method",
		"var s=\"a\"\nvar c=s[9]":        "out of range",
		"var m={}\nvar x=m[\"k\"]+1":     "unsupported types",
		"var a=[1,2]\nvar c=a[\"x\"]":    "not subscriptable",
		"var a=1\nvar b=[1]\nvar c=a[0]": "not subscriptable",
		"\"a\"-\"b\"":                    "unknown operator",
	}
	for src, wantPart := range cases {
		err := runErr(t, src)
		if err == nil {
			t.Fatalf("%q: expected an error", src)
		}
		if !strings.Contains(err.Error(), wantPart) {
			t.Fatalf("%q: want error containing %q, got %v", src, wantPart, err)
		}
	}
}

func TestUndefinedVariableIsCompileError(t *testing.T) {
	if errs := compileErrs("print(nosuchvar)"); errs == nil {
		t.Fatal("undefined variable should be a compile error")
	}
}

// ---------------------------------------------------------------------------
// 字符串转义 / 词法位置
// ---------------------------------------------------------------------------

func TestStringEscapes(t *testing.T) {
	cases := map[string]string{
		`"a\tb"`:       "a\tb",
		`"a\nb"`:       "a\nb",
		`"a\\b"`:       `a\b`,
		`"a\"b"`:       `a"b`,
		`"x"+"\t"+"y"`: "x\ty",
		`len("a\tb")`:  "3",
		`len("a\\b")`:  "3",
	}
	for src, want := range cases {
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%q: want %q, got %q", src, want, got)
		}
	}
}

func TestLexerTokenPositions(t *testing.T) {
	l := lexer.NewLexer("var x=1\ny")
	want := []struct {
		typ  string
		line int
		col  int
	}{
		{"Var", 1, 1},
		{"Ident", 1, 5},
		{"Assign", 1, 6},
		{"Int", 1, 7},
		{"LF", 1, 8},
		{"Ident", 2, 1},
	}
	for i, w := range want {
		tok := l.NextToken()
		if tok.Type != w.typ || tok.Loc.Line != w.line || tok.Loc.Column != w.col {
			t.Fatalf("token %d: want %s at %d:%d, got %s at %d:%d",
				i, w.typ, w.line, w.col, tok.Type, tok.Loc.Line, tok.Loc.Column)
		}
	}
}

func TestLexerEmptyAndEofAreSafe(t *testing.T) {
	l := lexer.NewLexer("")
	for i := 0; i < 3; i++ {
		if tok := l.NextToken(); !tok.IsEOF() {
			t.Fatalf("empty source should only produce EOF, got %s", tok.Str())
		}
	}
	if err := runErr(t, ""); err != nil {
		t.Fatalf("empty program should run cleanly, got %v", err)
	}
	if got := evalSrc(t, ""); got != "None" {
		t.Fatalf("empty program: want None, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// 解析器：语句分隔
// ---------------------------------------------------------------------------

// 表达式语句不能把下一行的第一个 token 吃掉。
func TestExprStatementDoesNotEatNextLine(t *testing.T) {
	src := `def g(){
	return 1
}
var i=0
for(;i<3;){
	g()
	i=i+1
}
i`
	if got := evalSrc(t, src); got != "3" {
		t.Fatalf("want 3, got %s", got)
	}
}

// 裸标识符作为块内最后一条语句也要能解析。
func TestBareIdentifierAsLastStatementInBlock(t *testing.T) {
	src := `var n=0
for(;n<2;){
	n=n+1
	n
}
n`
	if got := evalSrc(t, src); got != "2" {
		t.Fatalf("want 2, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// 全局变量槽位
// ---------------------------------------------------------------------------

// 全局槽位按需增长，初始容量之外也要能用。
func TestManyGlobalsBeyondInitialCapacity(t *testing.T) {
	const n = 1200
	var sb strings.Builder
	sb.WriteString("var last=0\n")
	for i := 0; i < n; i++ {
		sb.WriteString("var g")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("=")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("\n")
	}
	sb.WriteString("g0+g1+g")
	sb.WriteString(strconv.Itoa(n - 1))

	want := 0 + 1 + n - 1
	if got := evalSrc(t, sb.String()); got != strconv.Itoa(want) {
		t.Fatalf("want %d, got %s", want, got)
	}
}

// 未赋值的全局槽位读出来是 None，不能是 nil。
func TestUnsetGlobalReadsAsNone(t *testing.T) {
	src := `var a=1
def touch(){
	return none
}
touch()
type(a)`
	if got := evalSrc(t, src); got == "" {
		t.Fatal("type() should return something")
	}
}

// ---------------------------------------------------------------------------
// 运行时错误的行号定位
// ---------------------------------------------------------------------------

// 运行时错误必须报出源码行，且行号要对得上。
func TestRuntimeErrorReportsSourceLine(t *testing.T) {
	cases := []struct {
		name string
		src  string
		line int
	}{
		{"div0", "var a=1\nvar b=0\nvar c=a/b", 3},
		{"div0-first", "1/0", 1},
		{"index", "var arr=[1,2]\nvar x=arr[9]", 2},
		{"call-non-func", "var a=1\nvar b=2\na()", 3},
		{"bad-types", "var s=\"a\"\nvar n=s-1", 2},
		{"inside-func", "def f(x){\n\tvar y=x-1\n\treturn \"s\"/y\n}\nf(1)", 3},
	}
	for _, tc := range cases {
		_, err := evalOrErr(t, tc.src)
		if err == nil {
			t.Fatalf("%s: expected an error", tc.name)
		}
		if !strings.Contains(err.Error(), "line "+strconv.Itoa(tc.line)) {
			t.Fatalf("%s: want line %d in error, got %q", tc.name, tc.line, err.Error())
		}
	}
}

// 错误信息里要带函数名，便于定位。
func TestRuntimeErrorInsideFunctionNamesFunction(t *testing.T) {
	_, err := evalOrErr(t, "def bad(){\n\treturn 1/0\n}\nbad()")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Fatalf("error should name the function, got %q", err.Error())
	}
}

// 同一个错误不能被重复加上行号前缀。
func TestRuntimeErrorLineAnnotatedOnce(t *testing.T) {
	_, err := evalOrErr(t, "var s=\"a\"\nvar n=s-1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if n := strings.Count(err.Error(), "line "); n != 1 {
		t.Fatalf("line annotation should appear once, got %d in %q", n, err.Error())
	}
}

// ---------------------------------------------------------------------------
// 循环
// ---------------------------------------------------------------------------

// for(cond) 简写应当等同于 while 循环。
func TestWhileStyleForLoop(t *testing.T) {
	cases := map[string]string{
		"var i=0\nfor(i<3){\ni=i+1\n}\ni":                      "3",
		"for(1>2){10}\n7":                                      "7",
		"var i=0\nfor(i<10){\ni=i+1\nif(i>4){\nbreak\n}\n}\ni": "5",
	}
	for src, want := range cases {
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%q: want %s, got %s", src, want, got)
		}
	}
}

// 循环体里的纯表达式语句也不能泄漏栈槽。
func TestNoStackLeakForPureExpressionInLoop(t *testing.T) {
	src := `var i=0
for(;i<3000;){
	1+2
	i=i+1
}
i`
	if got := evalSrc(t, src); got != "3000" {
		t.Fatalf("want 3000, got %s", got)
	}
	if depth := evalMaxDepth(t, src); depth > 32 {
		t.Fatalf("stack should not grow per iteration, max depth = %d", depth)
	}
}

// ---------------------------------------------------------------------------
// 可重入 / 并发
// ---------------------------------------------------------------------------

func TestVMsAreIndependent(t *testing.T) {
	src := `var arr=[1,2,3]
var s=0
for(var i=0;i<200;i=i+1){
	s=s+len(arr)
}
s`
	const want = "600"
	var wg sync.WaitGroup
	errs := make([]error, 8)
	results := make([]string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := parser.NewParser(lexer.NewLexer(src))
			node := p.Parse()
			c := compiler.NewCompiler()
			c.SetSymbol(p.SymTable)
			c.Compile(node)
			if c.HasError() {
				errs[i] = c.Errs()[0]
				return
			}
			m := vm2.NewVM()
			if err := m.Run(c.ByteCode()); err != nil {
				errs[i] = err
				return
			}
			results[i] = m.LastPop().Inspect()
		}(i)
	}
	wg.Wait()
	for i := 0; i < 8; i++ {
		if errs[i] != nil {
			t.Fatalf("vm %d failed: %v", i, errs[i])
		}
		if results[i] != want {
			t.Fatalf("vm %d: want %s, got %s", i, want, results[i])
		}
	}
}

// ---------------------------------------------------------------------------
// 解释器模式（REPL）
// ---------------------------------------------------------------------------

func TestInterpreterModePrintsWithoutCorruptingStack(t *testing.T) {
	p := parser.NewParser(lexer.NewLexer("1+1\n2+2"))
	node := p.Parse()
	if p.HasError() {
		t.Fatalf("parse error: %v", p.Errs())
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.SetMode() // 打开 REPL 模式的隐式打印
	c.Compile(node)
	if c.HasError() {
		t.Fatalf("compile error: %v", c.Errs())
	}
	m := vm2.NewVM()
	if err := m.Run(c.ByteCode()); err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	if got := m.LastPop().Inspect(); got != "4" {
		t.Fatalf("want 4, got %s", got)
	}
}
