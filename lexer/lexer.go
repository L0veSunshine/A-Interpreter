// Package lexer 把源码切分成 token。
//
// 实现遵循几条通行的扫描器做法：
//
//  1. 输入统一成 []rune，索引是 rune 下标，因此位置信息对多字节字符也准确。
//  2. 换行在入口处归一化（CRLF/CR -> LF），行列号只依赖 '\n'。
//  3. EOF 用 -1 哨兵，源码中的 NUL 不会被误判为结束。
//  4. 运算符用"最长匹配表"（maximal munch）：按长度降序尝试，
//     新增运算符只需往表里加一行，不必改 NextToken 的分支。
//  5. 数字字面量在词法阶段归一化（0xFF -> 255），解析器只面对十进制，
//     不需要知道进制前缀。
//  6. 非法字符只消费"无法开始任何 token 的最短一段"，不会吞掉后面的合法代码。
//  7. 注释不吞行尾换行——换行是语句分隔符，吞掉会把相邻语句粘起来。
//  8. 错误带上 line/col 并设上限，畸形输入不会造成错误风暴。
package lexer

import (
	"fmt"
	"strconv"
	"strings"

	"Interpreter/errors"
	"Interpreter/tokens"
)

// maxErrors 限制词法错误数量，避免错误风暴本身成为内存/性能问题。
const maxErrors = 100

// Lexer 是源码扫描器。零值不可用，请用 NewLexer 构造。
type Lexer struct {
	src []rune
	// pos 是下一个待读 rune 的下标；line/col 描述 src[pos] 的位置（1 起）。
	pos  int
	line int
	col  int

	*errors.Errors
}

// NewLexer 构造扫描器。换行会被归一化为 '\n'。
func NewLexer(text string) *Lexer {
	return &Lexer{
		src:    []rune(normalizeNewlines(text)),
		line:   1,
		col:    1,
		Errors: errors.NewErr(),
	}
}

// normalizeNewlines 把 CRLF / CR 统一成 LF。
// 否则 Windows 换行的 \r 会被当空白跳过，而缺少 LF 会让两条语句粘在一起。
func normalizeNewlines(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// ---------------------------------------------------------------------------
// 游标
// ---------------------------------------------------------------------------

// peekChar 返回相对当前位置第 k 个字符（0 = 当前），越界返回 EOF。
func (l *Lexer) peekChar(k int) Char {
	i := l.pos + k
	if i < 0 || i >= len(l.src) {
		return EOF
	}
	return Char(l.src[i])
}

// advance 消费一个字符并返回它；到达末尾后是安全的空操作。
func (l *Lexer) advance() Char {
	c := l.peekChar(0)
	if c.IsEOF() {
		return EOF
	}
	if c.IsNewline() {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	l.pos++
	return c
}

// here 返回当前位置（下一个待读字符的位置）。
func (l *Lexer) here() tokens.Locate {
	return tokens.Locate{Line: l.line, Column: l.col, Offset: l.pos}
}

// Position 返回当前扫描位置，便于调用方报告"到某个 token 为止"的范围。
func (l *Lexer) Position() tokens.Locate { return l.here() }

// errorAt 报告一条带位置的词法错误。
func (l *Lexer) errorAt(loc tokens.Locate, format string, args ...interface{}) {
	if l.HasError() && len(l.Errs()) >= maxErrors {
		return
	}
	l.NewErrorF("line %d, col %d: %s", loc.Line, loc.Column, fmt.Sprintf(format, args...))
}

// ---------------------------------------------------------------------------
// 公共入口
// ---------------------------------------------------------------------------

// NextToken 返回下一个 token；到达末尾后持续返回 EOF token。
func (l *Lexer) NextToken() *tokens.Token {
	l.skipTrivia()
	start := l.here()
	c := l.peekChar(0)

	switch {
	case c.IsEOF():
		return tokenAt(tokens.EOF, "", start)
	case c.IsNewline():
		l.advance()
		return tokenAt(tokens.LF, "\n", start)
	case c.IsDigit(), c.Is('.') && l.peekChar(1).IsDigit():
		return l.scanNumber(start)
	case c.IsIdentStart():
		return l.scanIdentifier(start)
	case c.IsQuote():
		return l.scanString(start)
	}

	if typ, text, width, ok := l.matchOperator(); ok {
		for i := 0; i < width; i++ {
			l.advance()
		}
		return tokenAt(typ, text, start)
	}
	return l.scanIllegal(start)
}

// Tokenize 扫描整个输入，返回除 EOF 之外的全部 token。
func (l *Lexer) Tokenize() []tokens.Token {
	var out []tokens.Token
	for {
		tok := l.NextToken()
		if tok.IsEOF() {
			return out
		}
		out = append(out, *tok)
	}
}

func tokenAt(typ, literal string, loc tokens.Locate) *tokens.Token {
	return tokens.NToken(typ, literal, &loc)
}

// ---------------------------------------------------------------------------
// 空白与注释
// ---------------------------------------------------------------------------

func (l *Lexer) skipTrivia() {
	for {
		c := l.peekChar(0)
		switch {
		case c.IsEOF():
			return
		case c.IsSpace():
			l.advance()
		case c.Is('#'):
			l.skipComment()
		default:
			return
		}
	}
}

// skipComment 跳过 '#' 到行尾，但保留行尾换行。
// 换行是语句分隔符：吞掉它会把 "var a=1 # 注释\nb=2" 变成一条语句。
func (l *Lexer) skipComment() {
	for {
		c := l.peekChar(0)
		if c.IsEOF() || c.IsNewline() {
			return
		}
		l.advance()
	}
}

// ---------------------------------------------------------------------------
// 标识符与关键字
// ---------------------------------------------------------------------------

func (l *Lexer) scanIdentifier(start tokens.Locate) *tokens.Token {
	begin := l.pos
	for l.peekChar(0).IsIdentPart() {
		l.advance()
	}
	text := string(l.src[begin:l.pos])
	if typ, ok := tokens.Reserved[text]; ok {
		return tokenAt(typ, typ, start)
	}
	return tokenAt(tokens.Ident, text, start)
}

// ---------------------------------------------------------------------------
// 数字
// ---------------------------------------------------------------------------

func (l *Lexer) scanNumber(start tokens.Locate) *tokens.Token {
	if l.peekChar(0).Is('0') {
		switch l.peekChar(1).Rune() {
		case 'x', 'X':
			return l.scanRadixLiteral(start, 16, 2)
		case 'o', 'O':
			return l.scanRadixLiteral(start, 8, 2)
		case 'b', 'B':
			return l.scanRadixLiteral(start, 2, 2)
		}
	}

	var text []rune
	isFloat := false

	if l.peekChar(0).Is('.') {
		// .5 形式
		isFloat = true
		text = append(text, '0', '.')
		l.advance()
		text = append(text, l.scanDecimalDigits(start)...)
	} else {
		text = append(text, l.scanDecimalDigits(start)...)
		// 小数点只有后面紧跟数字时才算小数：`1.foo()` 里的点是成员访问
		if l.peekChar(0).Is('.') && l.peekChar(1).IsDigit() {
			isFloat = true
			text = append(text, '.')
			l.advance()
			text = append(text, l.scanDecimalDigits(start)...)
		}
	}

	// 指数部分：1e10 / 1.5E-3
	if c := l.peekChar(0); c.Is('e') || c.Is('E') {
		savePos, saveLine, saveCol := l.pos, l.line, l.col
		mark := len(text)
		text = append(text, 'e')
		l.advance()
		if s := l.peekChar(0); s.Is('+') || s.Is('-') {
			text = append(text, s.Rune())
			l.advance()
		}
		exp := l.scanDecimalDigits(start)
		if len(exp) == 0 {
			// 不构成指数：回退，"1e" 里的 e 应当留给标识符
			l.pos, l.line, l.col = savePos, saveLine, saveCol
			text = text[:mark]
		} else {
			isFloat = true
			text = append(text, exp...)
		}
	}

	if isFloat {
		return tokenAt(tokens.Float, string(text), start)
	}
	return tokenAt(tokens.Int, string(text), start)
}

// scanDecimalDigits 收集十进制数字，允许数字之间的单个下划线（1_000_000）。
// literal 里不含下划线，便于解析器直接 ParseInt。
func (l *Lexer) scanDecimalDigits(start tokens.Locate) []rune {
	var out []rune
	prevDigit := false
	for {
		c := l.peekChar(0)
		if c.Is('_') {
			if !prevDigit || !l.peekChar(1).IsDigit() {
				l.errorAt(start, " misplaced '_' in number literal")
				l.advance()
				continue
			}
			l.advance() // 跳过下划线，不写进 literal
			prevDigit = false
			continue
		}
		if !c.IsDigit() {
			return out
		}
		out = append(out, c.Rune())
		l.advance()
		prevDigit = true
	}
}

// scanRadixLiteral 处理 0x / 0o / 0b 前缀，把值归一化成十进制字面量。
func (l *Lexer) scanRadixLiteral(start tokens.Locate, base, prefixLen int) *tokens.Token {
	prefix := string(l.src[l.pos : l.pos+prefixLen])
	for i := 0; i < prefixLen; i++ {
		l.advance()
	}

	var digits []rune
	prevDigit := false
	for {
		c := l.peekChar(0)
		if c.Is('_') {
			if !prevDigit || l.peekChar(1).digitValue(base) < 0 {
				l.errorAt(start, " misplaced '_' in %s literal", prefix)
				l.advance()
				continue
			}
			l.advance()
			prevDigit = false
			continue
		}
		if c.digitValue(base) < 0 {
			if c.IsIdentPart() {
				l.errorAt(start, "invalid digit %s in %s literal", c.Quote(), prefix)
				l.advance()
				continue
			}
			break
		}
		digits = append(digits, c.Rune())
		l.advance()
		prevDigit = true
	}

	if len(digits) == 0 {
		l.errorAt(start, "%s literal has no digits", prefix)
		return tokenAt(tokens.Int, "0", start)
	}
	value, err := strconv.ParseUint(string(digits), base, 64)
	if err != nil {
		l.errorAt(start, "invalid %s literal %q: %v", prefix, prefix+string(digits), err)
		return tokenAt(tokens.Int, "0", start)
	}
	return tokenAt(tokens.Int, strconv.FormatUint(value, 10), start)
}

// ---------------------------------------------------------------------------
// 字符串
// ---------------------------------------------------------------------------

func (l *Lexer) scanString(start tokens.Locate) *tokens.Token {
	quote := l.advance().Rune()
	var out []rune
	terminated := false

	for {
		c := l.peekChar(0)
		if c.IsEOF() {
			break
		}
		if c.Is(quote) {
			l.advance()
			terminated = true
			break
		}
		if c.Is('\\') {
			l.advance()
			if r, ok := l.scanEscape(start); ok {
				out = append(out, r)
			}
			continue
		}
		out = append(out, c.Rune())
		l.advance()
	}

	if !terminated {
		l.errorAt(start, "unterminated string literal")
	}
	return tokenAt(tokens.String, string(out), start)
}

// scanEscape 解析转义序列（反斜杠已消费）。ok=false 表示该转义已报错。
func (l *Lexer) scanEscape(start tokens.Locate) (rune, bool) {
	c := l.peekChar(0)
	if c.IsEOF() {
		l.errorAt(start, "unfinished escape sequence at end of input")
		return 0, false
	}
	l.advance()

	switch c.Rune() {
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'r':
		return '\r', true
	case '0':
		return 0, true
	case '\\':
		return '\\', true
	case '"':
		return '"', true
	case '\'':
		return '\'', true
	case 'x':
		return l.scanHexEscape(start, 2)
	case 'u':
		return l.scanHexEscape(start, 4)
	}
	l.errorAt(start, "unknown escape sequence %s", c.Quote())
	return c.Rune(), true
}

// scanHexEscape 读取 n 个十六进制数字组成一个码点。
func (l *Lexer) scanHexEscape(start tokens.Locate, n int) (rune, bool) {
	var digits []rune
	for i := 0; i < n; i++ {
		c := l.peekChar(0)
		if !c.IsHexDigit() {
			l.errorAt(start, "escape sequence needs %d hex digits but got %s", n, c.Quote())
			return 0, false
		}
		digits = append(digits, c.Rune())
		l.advance()
	}
	value, err := strconv.ParseUint(string(digits), 16, 32)
	if err != nil {
		l.errorAt(start, "invalid hex escape %q: %v", string(digits), err)
		return 0, false
	}
	r := rune(value)
	if !isValidRune(r) {
		l.errorAt(start, "escape sequence %q is not a valid character", string(digits))
		return 0, false
	}
	return r, true
}

func isValidRune(r rune) bool {
	return r >= 0 && r <= 0x10FFFF && !(r >= 0xD800 && r <= 0xDFFF)
}

// ---------------------------------------------------------------------------
// 运算符（最长匹配）
// ---------------------------------------------------------------------------

// operator 是运算符表的一行。表按 text 长度降序排列，
// matchOperator 从长到短尝试，因此天然实现 maximal munch。
type operator struct {
	text  string
	typ   string
	runes []rune
}

var operators = buildOperators([]operator{
	// 3 字符
	{text: "**=", typ: tokens.IPow},
	{text: "//=", typ: tokens.IFloorDiv},
	// 2 字符
	{text: "==", typ: tokens.Equal},
	{text: "!=", typ: tokens.NotEq},
	{text: "<=", typ: tokens.LTEq},
	{text: ">=", typ: tokens.GTEq},
	{text: "+=", typ: tokens.IPlus},
	{text: "-=", typ: tokens.IMinus},
	{text: "*=", typ: tokens.IMul},
	{text: "/=", typ: tokens.IDiv},
	{text: "%=", typ: tokens.IMod},
	{text: "**", typ: tokens.Pow},
	{text: "//", typ: tokens.FloorDiv},
	// 1 字符
	{text: "=", typ: tokens.Assign},
	{text: "<", typ: tokens.LT},
	{text: ">", typ: tokens.GT},
	{text: "!", typ: tokens.Not},
	{text: "+", typ: tokens.Plus},
	{text: "-", typ: tokens.Minus},
	{text: "*", typ: tokens.Mul},
	{text: "/", typ: tokens.Div},
	{text: "%", typ: tokens.Mod},
	{text: "(", typ: tokens.LParen},
	{text: ")", typ: tokens.RParen},
	{text: "[", typ: tokens.LBRACKET},
	{text: "]", typ: tokens.RBRACKET},
	{text: "{", typ: tokens.LBRACE},
	{text: "}", typ: tokens.RBRACE},
	{text: ".", typ: tokens.Dot},
	{text: ":", typ: tokens.Colon},
	{text: ",", typ: tokens.Comma},
	{text: ";", typ: tokens.Semi},
})

// operatorFirst 记录所有运算符的首字符，供非法字符扫描判断"这里是否还能开始一个 token"。
var operatorFirst = map[rune]bool{}

func buildOperators(list []operator) []operator {
	for i := range list {
		list[i].runes = []rune(list[i].text)
	}
	for _, op := range list {
		operatorFirst[op.runes[0]] = true
	}
	return list
}

// matchOperator 返回当前位置能匹配的最长运算符。
func (l *Lexer) matchOperator() (typ, text string, width int, ok bool) {
	for _, op := range operators {
		if l.matchesRunes(op.runes) {
			return op.typ, op.text, len(op.runes), true
		}
	}
	return "", "", 0, false
}

func (l *Lexer) matchesRunes(want []rune) bool {
	for i, r := range want {
		if l.peekChar(i).Rune() != r {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// 非法字符
// ---------------------------------------------------------------------------

// scanIllegal 消费"无法开始任何 token 的一段字符"并报错。
// 只吃掉这段、而不是"直到空白"，避免把后面的合法代码一起吞掉
// （原实现里 "@a" 会把 a 也当成非法 token 的一部分）。
func (l *Lexer) scanIllegal(start tokens.Locate) *tokens.Token {
	begin := l.pos
	for {
		c := l.peekChar(0)
		if c.IsEOF() || c.IsSpace() || c.IsNewline() || c.Is('#') || startsToken(c) {
			break
		}
		l.advance()
	}
	if l.pos == begin { // 兜底：至少消费一个字符，保证扫描一定前进
		l.advance()
	}
	text := string(l.src[begin:l.pos])
	l.errorAt(start, "illegal character %q", text)
	return tokenAt(tokens.Illegal, text, start)
}

// startsToken 判断该字符能否作为某个 token 的开头。
func startsToken(c Char) bool {
	if c.IsEOF() || c.IsNewline() || c.IsSpace() {
		return true
	}
	if c.IsDigit() || c.IsIdentStart() || c.IsQuote() {
		return true
	}
	return operatorFirst[c.Rune()]
}
