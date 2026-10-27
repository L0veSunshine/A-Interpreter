package lexer

import (
	"strings"
	"testing"

	"Interpreter/tokens"
)

// tokenize 是测试辅助：扫描全部 token。
func tokenize(src string) []tokens.Token {
	return NewLexer(src).Tokenize()
}

func tokenizeErr(src string) ([]tokens.Token, []error) {
	l := NewLexer(src)
	toks := l.Tokenize()
	return toks, l.Errs()
}

// kinds 返回 token 类型序列，便于断言。
func kinds(toks []tokens.Token) []string {
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = t.Type
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// 基本 token 流
// ---------------------------------------------------------------------------

func TestTokenizeBasic(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"1", []string{tokens.Int}},
		{"1.5", []string{tokens.Float}},
		{"abc", []string{tokens.Ident}},
		{`"s"`, []string{tokens.String}},
		{"true false none", []string{tokens.True, tokens.False, tokens.None}},
		{"var for def if else return break", []string{
			tokens.Var, tokens.For, tokens.Func, tokens.If, tokens.Else, tokens.Return, tokens.Break,
		}},
		{"a+b", []string{tokens.Ident, tokens.Plus, tokens.Ident}},
	}
	for _, tc := range cases {
		if got := kinds(tokenize(tc.src)); !equalStrings(got, tc.want) {
			t.Fatalf("%q: want %v, got %v", tc.src, tc.want, got)
		}
	}
}

// 每个运算符都必须被识别，且最长匹配优先。
func TestAllOperatorsMaximalMunch(t *testing.T) {
	cases := map[string]string{
		"=":   tokens.Assign,
		"==":  tokens.Equal,
		"!=":  tokens.NotEq,
		"!":   tokens.Not,
		"<":   tokens.LT,
		"<=":  tokens.LTEq,
		">":   tokens.GT,
		">=":  tokens.GTEq,
		"+":   tokens.Plus,
		"+=":  tokens.IPlus,
		"-":   tokens.Minus,
		"-=":  tokens.IMinus,
		"*":   tokens.Mul,
		"*=":  tokens.IMul,
		"**":  tokens.Pow,
		"**=": tokens.IPow,
		"/":   tokens.Div,
		"/=":  tokens.IDiv,
		"%":   tokens.Mod,
		"%=":  tokens.IMod,
		"(":   tokens.LParen,
		")":   tokens.RParen,
		"[":   tokens.LBRACKET,
		"]":   tokens.RBRACKET,
		"{":   tokens.LBRACE,
		"}":   tokens.RBRACE,
		".":   tokens.Dot,
		":":   tokens.Colon,
		",":   tokens.Comma,
		";":   tokens.Semi,
	}
	for src, want := range cases {
		toks := tokenize(src)
		if len(toks) != 1 {
			t.Fatalf("%q: want exactly 1 token, got %v", src, kinds(toks))
		}
		if toks[0].Type != want {
			t.Fatalf("%q: want %s, got %s", src, want, toks[0].Type)
		}
	}
}

// 最长匹配：`a**=b` 不能切成 `*` `*` `=`。
func TestMaximalMunchLongestWins(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"a**=b", []string{tokens.Ident, tokens.IPow, tokens.Ident}},
		{"a**b", []string{tokens.Ident, tokens.Pow, tokens.Ident}},
		{"a*=b", []string{tokens.Ident, tokens.IMul, tokens.Ident}},
		{"a==b", []string{tokens.Ident, tokens.Equal, tokens.Ident}},
		{"a=b", []string{tokens.Ident, tokens.Assign, tokens.Ident}},
	}
	for _, tc := range cases {
		if got := kinds(tokenize(tc.src)); !equalStrings(got, tc.want) {
			t.Fatalf("%q: want %v, got %v", tc.src, tc.want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 位置信息
// ---------------------------------------------------------------------------

func TestTokenPositions(t *testing.T) {
	src := "var x=1\ny"
	want := []struct {
		typ       string
		line, col int
		offset    int
	}{
		{tokens.Var, 1, 1, 0},
		{tokens.Ident, 1, 5, 4},
		{tokens.Assign, 1, 6, 5},
		{tokens.Int, 1, 7, 6},
		{tokens.LF, 1, 8, 7},
		{tokens.Ident, 2, 1, 8},
	}
	toks := tokenize(src)
	if len(toks) != len(want) {
		t.Fatalf("want %d tokens, got %d: %v", len(want), len(toks), kinds(toks))
	}
	for i, w := range want {
		got := toks[i]
		if got.Type != w.typ || got.Loc.Line != w.line || got.Loc.Column != w.col || got.Loc.Offset != w.offset {
			t.Fatalf("token %d: want %s@%d:%d(off %d), got %s@%d:%d(off %d)",
				i, w.typ, w.line, w.col, w.offset,
				got.Type, got.Loc.Line, got.Loc.Column, got.Loc.Offset)
		}
	}
}

// 前导小数点的小数，位置必须指向小数点本身（原来指向 token 结束处）。
func TestLeadingDotFloatPosition(t *testing.T) {
	toks := tokenize("x=.5")
	if len(toks) != 3 {
		t.Fatalf("want 3 tokens, got %v", kinds(toks))
	}
	f := toks[2]
	if f.Type != tokens.Float || f.Literal != "0.5" {
		t.Fatalf("want Float 0.5, got %s %q", f.Type, f.Literal)
	}
	if f.Loc.Column != 3 || f.Loc.Offset != 2 {
		t.Fatalf("float should start at col 3 / offset 2, got col %d / offset %d",
			f.Loc.Column, f.Loc.Offset)
	}
}

// 多字节字符不应让列号错位。
func TestUnicodeColumnAccuracy(t *testing.T) {
	toks := tokenize(`"中文"+1`)
	if len(toks) != 3 {
		t.Fatalf("want 3 tokens, got %v", kinds(toks))
	}
	if got := toks[1].Loc.Column; got != 5 {
		t.Fatalf(`"中文" is 4 runes wide, so "+" should be at col 5, got %d`, got)
	}
	if got := toks[2].Loc.Column; got != 6 {
		t.Fatalf("digit should be at col 6, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// 换行：CRLF / CR 归一化，注释不吞换行
// ---------------------------------------------------------------------------

func TestCRLFNormalized(t *testing.T) {
	for _, src := range []string{"a\nb", "a\r\nb", "a\rb"} {
		toks := tokenize(src)
		want := []string{tokens.Ident, tokens.LF, tokens.Ident}
		if got := kinds(toks); !equalStrings(got, want) {
			t.Fatalf("%q: want %v, got %v", src, want, got)
		}
		if toks[2].Loc.Line != 2 || toks[2].Loc.Column != 1 {
			t.Fatalf("%q: second ident should be at 2:1, got %d:%d",
				src, toks[2].Loc.Line, toks[2].Loc.Column)
		}
	}
}

// 注释不能吞掉行尾换行，否则相邻两条语句会粘在一起。
func TestCommentKeepsNewline(t *testing.T) {
	src := "var a=1 # 注释\nb=2"
	toks := tokenize(src)
	want := []string{
		tokens.Var, tokens.Ident, tokens.Assign, tokens.Int, tokens.LF,
		tokens.Ident, tokens.Assign, tokens.Int,
	}
	if got := kinds(toks); !equalStrings(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	if toks[5].Loc.Line != 2 {
		t.Fatalf("b should be on line 2, got %d", toks[5].Loc.Line)
	}
}

func TestCommentAtEOFAndEmptyComment(t *testing.T) {
	for _, src := range []string{"# 只有注释", "a #x", "#\n", "#\n#\n"} {
		if _, errs := tokenizeErr(src); len(errs) != 0 {
			t.Fatalf("%q: unexpected errors %v", src, errs)
		}
	}
	if got := kinds(tokenize("# 只有注释")); len(got) != 0 {
		t.Fatalf("comment-only input should produce no tokens, got %v", got)
	}
	if got := kinds(tokenize("a #x")); !equalStrings(got, []string{tokens.Ident}) {
		t.Fatalf("want [Ident], got %v", got)
	}
}

// ---------------------------------------------------------------------------
// 数字字面量
// ---------------------------------------------------------------------------

func TestNumberLiterals(t *testing.T) {
	cases := []struct {
		src string
		typ string
		lit string
	}{
		{"0", tokens.Int, "0"},
		{"123", tokens.Int, "123"},
		{"1_000_000", tokens.Int, "1000000"},
		{"0xFF", tokens.Int, "255"},
		{"0XfF", tokens.Int, "255"},
		{"0o17", tokens.Int, "15"},
		{"0b1010", tokens.Int, "10"},
		{"1.5", tokens.Float, "1.5"},
		{".5", tokens.Float, "0.5"},
		{"1.25", tokens.Float, "1.25"},
		{"1e3", tokens.Float, "1e3"},
		{"1E-3", tokens.Float, "1e-3"}, // 指数标记统一成小写 e
		{"1.5e+10", tokens.Float, "1.5e+10"},
	}
	for _, tc := range cases {
		toks, errs := tokenizeErr(tc.src)
		if len(errs) != 0 {
			t.Fatalf("%q: unexpected errors %v", tc.src, errs)
		}
		if len(toks) != 1 {
			t.Fatalf("%q: want 1 token, got %v", tc.src, kinds(toks))
		}
		if toks[0].Type != tc.typ || toks[0].Literal != tc.lit {
			t.Fatalf("%q: want %s %q, got %s %q", tc.src, tc.typ, tc.lit, toks[0].Type, toks[0].Literal)
		}
	}
}

// 数字后面紧跟标识符时，不能被吞进数字里。
func TestNumberFollowedByIdentifier(t *testing.T) {
	toks := tokenize("12abc")
	want := []string{tokens.Int, tokens.Ident}
	if got := kinds(toks); !equalStrings(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	if toks[0].Literal != "12" || toks[1].Literal != "abc" {
		t.Fatalf("got %q %q", toks[0].Literal, toks[1].Literal)
	}
}

// `1.foo()` 里的点是成员访问，不是小数点。
func TestDotAfterNumberIsNotFloat(t *testing.T) {
	toks := tokenize("1.foo")
	want := []string{tokens.Int, tokens.Dot, tokens.Ident}
	if got := kinds(toks); !equalStrings(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestInvalidNumbersReportErrors(t *testing.T) {
	cases := []string{
		"1__0", // 连续下划线
		"1_",   // 结尾下划线
		"0x",   // 无数字
		"0b12", // 2 进制里的非法数字
		"0o8",  // 8 进制里的非法数字
		"0xGG", // 无有效数字
		"1e",   // 缺指数（应回退成 Int + Ident，不报错）
	}
	for _, src := range cases {
		_, errs := tokenizeErr(src)
		if src == "1e" {
			// 允许：回退为 Int(1) + Ident(e)
			if len(errs) != 0 {
				t.Fatalf("%q: should fall back without error, got %v", src, errs)
			}
			continue
		}
		if len(errs) == 0 {
			t.Fatalf("%q: expected a lexical error", src)
		}
	}
}

// ---------------------------------------------------------------------------
// 字符串
// ---------------------------------------------------------------------------

func TestStringEscapes(t *testing.T) {
	cases := map[string]string{
		`"a\tb"`:   "a\tb",
		`"a\nb"`:   "a\nb",
		`"a\rb"`:   "a\rb",
		`"a\\b"`:   `a\b`,
		`"a\"b"`:   `a"b`,
		`'a\'b'`:   `a'b`,
		`"\x41"`:   "A",
		`"\u4e2d"`: "中",
		`"\0"`:     "\x00",
		`"中文"`:     "中文",
		`'单引号'`:    "单引号",
		`""`:       "",
	}
	for src, want := range cases {
		toks, errs := tokenizeErr(src)
		if len(errs) != 0 {
			t.Fatalf("%q: unexpected errors %v", src, errs)
		}
		if len(toks) != 1 || toks[0].Type != tokens.String {
			t.Fatalf("%q: want one String token, got %v", src, kinds(toks))
		}
		if toks[0].Literal != want {
			t.Fatalf("%q: want %q, got %q", src, want, toks[0].Literal)
		}
	}
}

func TestStringErrors(t *testing.T) {
	cases := []string{
		`"unterminated`,
		`'unterminated`,
		`"bad \q escape"`,
		`"\x4"`,  // hex 位数不足
		`"\u12"`, // 位数不足
		`"end\`,  // 末尾反斜杠
	}
	for _, src := range cases {
		_, errs := tokenizeErr(src)
		if len(errs) == 0 {
			t.Fatalf("%q: expected a lexical error", src)
		}
	}
	// 未闭合的字符串仍要返回 token，解析器才能继续工作
	toks := tokenize(`"abc`)
	if len(toks) != 1 || toks[0].Type != tokens.String {
		t.Fatalf("unterminated string should still yield a String token, got %v", kinds(toks))
	}
}

// 字符串里允许换行，行号要继续累加。
func TestMultilineString(t *testing.T) {
	toks, errs := tokenizeErr("\"a\nb\"\nc")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors %v", errs)
	}
	want := []string{tokens.String, tokens.LF, tokens.Ident}
	if got := kinds(toks); !equalStrings(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	if toks[0].Literal != "a\nb" {
		t.Fatalf("literal = %q", toks[0].Literal)
	}
	if toks[2].Loc.Line != 3 {
		t.Fatalf("c should be on line 3, got %d", toks[2].Loc.Line)
	}
}

// ---------------------------------------------------------------------------
// 健壮性
// ---------------------------------------------------------------------------

// NUL 是合法字符，不能被当成 EOF 而丢弃后面的代码。
func TestNULByteDoesNotEndInput(t *testing.T) {
	toks := tokenize("a\x00b")
	want := []string{tokens.Ident, tokens.Illegal, tokens.Ident}
	if got := kinds(toks); !equalStrings(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	if toks[2].Literal != "b" {
		t.Fatalf("token after NUL should be %q, got %q", "b", toks[2].Literal)
	}
}

// 非法字符不能吞掉后面的合法 token。
func TestIllegalCharacterDoesNotSwallowCode(t *testing.T) {
	toks := tokenize("a @b")
	want := []string{tokens.Ident, tokens.Illegal, tokens.Ident}
	if got := kinds(toks); !equalStrings(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	if toks[1].Literal != "@" {
		t.Fatalf("illegal literal should be %q, got %q", "@", toks[1].Literal)
	}
	if toks[2].Literal != "b" {
		t.Fatalf("b must survive, got %q", toks[2].Literal)
	}
}

func TestRepeatedEOFIsSafe(t *testing.T) {
	l := NewLexer("")
	for i := 0; i < 5; i++ {
		tok := l.NextToken()
		if !tok.IsEOF() {
			t.Fatalf("call %d: want EOF, got %s", i, tok.Type)
		}
	}
}

func TestTokenizeEmpty(t *testing.T) {
	for _, src := range []string{"", "   ", "# c", "\t\t"} {
		if toks := tokenize(src); len(toks) != 0 {
			t.Fatalf("%q: want no tokens, got %v", src, kinds(toks))
		}
	}
	// 换行是语句分隔符，必须保留
	if got := kinds(tokenize("\n\n")); !equalStrings(got, []string{tokens.LF, tokens.LF}) {
		t.Fatalf("blank lines should still yield LF tokens, got %v", got)
	}
}

// 错误数量必须有上限，且错误信息带 line/col。
func TestErrorsBoundedAndPositioned(t *testing.T) {
	src := strings.Repeat("@@@", 200)
	_, errs := tokenizeErr(src)
	if len(errs) == 0 {
		t.Fatal("expected lexical errors")
	}
	if len(errs) > maxErrors {
		t.Fatalf("error count should be capped at %d, got %d", maxErrors, len(errs))
	}
	if !strings.Contains(errs[0].Error(), "line 1") || !strings.Contains(errs[0].Error(), "col 1") {
		t.Fatalf("error should carry line/col, got %q", errs[0].Error())
	}
}

// 任意输入都必须终止，且不 panic。
func TestArbitraryInputTerminates(t *testing.T) {
	inputs := []string{
		"\"", "'", "\\", "#", "\x00", "\xff",
		strings.Repeat("\"", 50),
		strings.Repeat("{", 100),
		"0x" + strings.Repeat("f", 100),
		strings.Repeat("1_", 100),
		"a" + strings.Repeat("_", 100),
	}
	for _, src := range inputs {
		done := make(chan struct{})
		go func(src string) {
			defer close(done)
			NewLexer(src).Tokenize()
		}(src)
		<-done
	}
}

// ---------------------------------------------------------------------------
// 与其他组件的一致性
// ---------------------------------------------------------------------------

// 关键字必须映射到对应的 token 类型（不是 Ident）。
func TestKeywordsAreReserved(t *testing.T) {
	for word, typ := range tokens.Reserved {
		toks := tokenize(word)
		if len(toks) != 1 {
			t.Fatalf("%q: want 1 token, got %v", word, kinds(toks))
		}
		if toks[0].Type != typ {
			t.Fatalf("%q: want %s, got %s", word, typ, toks[0].Type)
		}
	}
	// 前缀相同但不是关键字的标识符仍是 Ident
	for _, word := range []string{"vars", "iff", "forest", "returns", "breaks", "definition"} {
		if toks := tokenize(word); toks[0].Type != tokens.Ident {
			t.Fatalf("%q should be Ident, got %s", word, toks[0].Type)
		}
	}
}

func TestPositionMethodTracksCursor(t *testing.T) {
	l := NewLexer("ab")
	if got := l.Position(); got.Offset != 0 || got.Line != 1 || got.Column != 1 {
		t.Fatalf("initial position = %+v", got)
	}
	l.NextToken()
	if got := l.Position(); got.Offset != 2 || got.Column != 3 {
		t.Fatalf("position after ident = %+v", got)
	}
}

// ---------------------------------------------------------------------------
// 基准
// ---------------------------------------------------------------------------

func BenchmarkTokenize(b *testing.B) {
	src := `
def fib(x){
	if(x<=1){
		return x
	}
	var cache={}
	cache[x]=fib(x-1)+fib(x-2)
	return cache[x]
}
var i=0
var sum=0
for(;i<100;i+=1){
	sum=sum+fib(10)
}
print(sum)`
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		NewLexer(src).Tokenize()
	}
}
