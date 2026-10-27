package main

import (
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/parser"
	vm2 "Interpreter/vm"
	"strings"
	"testing"
	"time"
)

// 解析器必须对任意输入终止：语法错误时不能让循环原地踏步，
// 更不能无限制地累积错误（曾经因为 parseMap 缺 EOF 检查而 OOM）。
func TestParserTerminatesOnMalformedInput(t *testing.T) {
	cases := []string{
		"{",
		"{{{{",
		"{a",
		"{a:",
		"{a:1",
		"{:1}",
		"if(true){",
		"if(true){a=1",
		"for(;;){",
		"for(",
		"def f(",
		"def f(){",
		"else{1}",
		"a[",
		"a[1",
		"a.",
		"a.b(",
		"(",
		")",
		"}",
		"]]]",
		"\"unterminated",
		"'unterminated",
		"var",
		"var x",
		"var x=",
		"return",
		"break",
		"1+",
		"*",
		"#comment only",
		strings.Repeat("{", 200),
		strings.Repeat("if(true){", 100),
		strings.Repeat("(", 500),
		strings.Repeat("a[", 300),
		"{\n" + strings.Repeat("a:1,\n", 500),
	}

	for _, src := range cases {
		done := make(chan interface{}, 1)
		go func(src string) {
			defer func() {
				if r := recover(); r != nil {
					done <- r
				}
			}()
			p := parser.NewParser(lexer.NewLexer(src))
			node := p.Parse()
			_ = node.Str()
			if p.HasError() {
				done <- nil // 语法错误时不应执行，只要求解析本身终止
				return
			}
			c := compiler.NewCompiler()
			c.SetSymbol(p.SymTable)
			c.Compile(node)
			if c.HasError() {
				done <- nil
				return
			}
			m := vm2.NewVM()
			_ = m.Run(c.ByteCode())
			done <- nil
		}(src)
		select {
		case r := <-done:
			if r != nil {
				t.Fatalf("input %q panicked: %v", truncate(src), r)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("parsing %q did not terminate", truncate(src))
		}
	}
}

func truncate(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

// 错误数量必须有上界，避免错误风暴把内存吃光。
func TestErrorCountIsBounded(t *testing.T) {
	src := strings.Repeat("{{{{", 200) + strings.Repeat("}}}}", 200)
	p := parser.NewParser(lexer.NewLexer(src))
	p.Parse()
	if n := len(p.Errs()); n > 500 {
		t.Fatalf("error count should be bounded, got %d", n)
	}
}

// 语法错误时编译器不应该崩溃：真实调用方可能忘记检查 HasError。
func TestCompilerSurvivesInvalidAST(t *testing.T) {
	cases := []string{"1+", "*", "a[", "var", "var x=", "a.b(", "if(true){", "{a:"}
	for _, src := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("compiling %q panicked: %v", src, r)
				}
			}()
			p := parser.NewParser(lexer.NewLexer(src))
			node := p.Parse()
			c := compiler.NewCompiler()
			c.SetSymbol(p.SymTable)
			c.Compile(node)
			bc := c.ByteCode()
			if c.HasError() {
				return
			}
			// 即使没有报错，也不能在合法字节码上崩溃
			m := vm2.NewVM()
			_ = m.Run(bc)
		}()
	}
}

// None 是假值，与 if/for 的条件判断一致。
func TestNoneIsFalsy(t *testing.T) {
	cases := map[string]string{
		`if(none){1}else{2}`: "2",
		`not none`:           "true",
		`none and true`:      "false",
		`none or true`:       "true",
		`"a" and true`:       "true",
		`"" and true`:        "false",
	}
	for src, want := range cases {
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%q: want %s, got %s", src, want, got)
		}
	}
}

// for(;;) 这类无显式条件的循环等价于恒真，靠 break 退出。
func TestForWithoutConditionIsInfiniteUntilBreak(t *testing.T) {
	src := `var i=0
for(;;){
	i=i+1
	if(i==4){
		break
	}
}
i`
	if got := evalSrc(t, src); got != "4" {
		t.Fatalf("want 4, got %s", got)
	}
}

// 合法的 map 字面量（含换行、多条目、空 map）要继续正常工作。
func TestMapLiteralsStillWork(t *testing.T) {
	cases := map[string]string{
		"{}":                     "{}",
		"{\n1:2\n}":              "{1: 2}",
		"{\n1:2,\n3:4\n}":        "{1: 2, 3: 4}",
		"var m={1:2}\nm[3]=4\nm": "{1: 2, 3: 4}",
	}
	for src, want := range cases {
		if got := evalSrc(t, src); got != want {
			t.Fatalf("%q: want %s, got %s", src, want, got)
		}
	}
}

// else 换行写也要能正确绑定到 if。
func TestElseOnNextLine(t *testing.T) {
	src := `var a=0
if(false){
	a=1
}
else{
	a=2
}
a`
	if got := evalSrc(t, src); got != "2" {
		t.Fatalf("want 2, got %s", got)
	}
}

// 没有 else 时，if 之后的下一条语句不能被吞掉。
func TestStatementAfterIfIsPreserved(t *testing.T) {
	src := `var a=1
if(true){
	a=2
}
a=a+10
a`
	if got := evalSrc(t, src); got != "12" {
		t.Fatalf("want 12, got %s", got)
	}
	b := compileSrcBytes(t, src)
	if len(b) == 0 {
		t.Fatal("expected bytecode")
	}
}

func compileSrcBytes(t *testing.T, src string) []byte {
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
	return c.ByteCode().Instruction
}
