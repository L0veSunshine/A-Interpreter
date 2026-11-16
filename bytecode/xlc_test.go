package bytecode_test

import (
	"strings"
	"testing"

	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/compiler"
	"Interpreter/lexer"
	"Interpreter/object"
	"Interpreter/parser"
)

const sampleSrc = `
def fib(n){
	if(n < 2){
		return n
	}
	return fib(n - 1) + fib(n - 2)
}
var m = {1: 2, 3: 4}
var a = [1, 2.5, "s", true, none]
fib(10)
`

func compileSample(t *testing.T, optimize bool) *bytecode.Bytecode {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(sampleSrc))
	node := p.Parse()
	if p.HasError() {
		t.Fatalf("parse: %v", p.Errs())
	}
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.SetOptimize(optimize)
	c.Compile(node)
	if c.HasError() {
		t.Fatalf("compile: %v", c.Errs())
	}
	return c.ByteCode()
}

func encodeSample(t *testing.T, optimize bool) ([]byte, bytecode.CacheMeta) {
	t.Helper()
	meta := bytecode.CacheMeta{
		Optimized:   optimize,
		SourceHash:  bytecode.SourceHash([]byte(sampleSrc)),
		SourceSize:  int64(len(sampleSrc)),
		SourceMTime: 1700000000000000000,
	}
	blob, err := bytecode.Encode(compileSample(t, optimize), meta)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return blob, meta
}

func TestCacheRoundTrip(t *testing.T) {
	for _, optimize := range []bool{true, false} {
		blob, wantMeta := encodeSample(t, optimize)
		bc, meta, err := bytecode.Decode(blob)
		if err != nil {
			t.Fatalf("decode(optimize=%v): %v", optimize, err)
		}
		if meta != wantMeta {
			t.Fatalf("meta roundtrip: want %+v, got %+v", wantMeta, meta)
		}
		if err := bytecode.Validate(bc); err != nil {
			t.Fatalf("loaded bytecode should be valid: %v", err)
		}
		// 常量池逐项对照：类型必须一致，可打印的值内容也必须一致
		// （函数对象的 Inspect 是地址，跳过内容比较，下面单独检查字段）。
		orig := compileSample(t, optimize)
		if len(bc.Constants) != len(orig.Constants) {
			t.Fatalf("constant count: want %d, got %d", len(orig.Constants), len(bc.Constants))
		}
		for i, want := range orig.Constants {
			got := bc.Constants[i]
			if got.Type() != want.Type() {
				t.Fatalf("constant %d: type %s != %s", i, got.Type(), want.Type())
			}
			if _, isFunc := want.(object.CompiledFunc); !isFunc && got.Inspect() != want.Inspect() {
				t.Fatalf("constant %d: %q != %q", i, got.Inspect(), want.Inspect())
			}
		}
		// 函数对象的关键字段
		found := false
		for _, c := range bc.Constants {
			fn, ok := c.(object.CompiledFunc)
			if !ok || fn.FnName != "fib" {
				continue
			}
			found = true
			if fn.ParametersNum != 1 || fn.LocalsNum < 1 {
				t.Fatalf("fib signature lost: params=%d locals=%d", fn.ParametersNum, fn.LocalsNum)
			}
			if len(fn.Instructions) == 0 || len(fn.Lines) == 0 {
				t.Fatalf("fib body lost: %d instructions, %d line entries",
					len(fn.Instructions), len(fn.Lines))
			}
		}
		if !found {
			t.Fatal("fib function object missing after roundtrip")
		}
		// 主函数体与行号表
		if len(bc.Instruction) == 0 || len(bc.Lines) == 0 {
			t.Fatalf("main body lost: %d bytes, %d line entries", len(bc.Instruction), len(bc.Lines))
		}
	}
}

// 方法名表要带回来：OpCallMethod 的操作数是它的下标。
func TestCacheKeepsMethodNames(t *testing.T) {
	const src = `var a = [1]
a.append(2)
a`
	p := parser.NewParser(lexer.NewLexer(src))
	node := p.Parse()
	c := compiler.NewCompiler()
	c.SetSymbol(p.SymTable)
	c.Compile(node)
	blob, err := bytecode.Encode(c.ByteCode(), bytecode.CacheMeta{SourceHash: bytecode.SourceHash([]byte(src))})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	bc, _, err := bytecode.Decode(blob)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	names := bc.Symbols.Methods.Names()
	if len(names) != 1 || names[0] != "append" {
		t.Fatalf("method names lost: %v", names)
	}
}

func TestDecodeRejectsBadInput(t *testing.T) {
	good, _ := encodeSample(t, true)

	cases := map[string][]byte{
		"empty":       {},
		"wrong magic": append([]byte("NOPE"), good[4:]...),
		"truncated":   good[:len(good)/2],
		"half header": good[:6],
		"garbage":     []byte(strings.Repeat("\x01", 200)),
	}
	for name, data := range cases {
		if _, _, err := bytecode.Decode(data); err == nil {
			t.Fatalf("%s: decode should fail", name)
		}
	}
	// 版本号被改过时必须拒绝
	bad := append([]byte(nil), good...)
	bad[4] = 0x7f
	bad[5] = 0x7f
	if _, _, err := bytecode.Decode(bad); err == nil {
		t.Fatal("version mismatch should be rejected")
	}
}

// 校验层要挡住"结构合法但跳到界外 / 操作码未知 / 常量下标越界"的字节码。
func TestValidateRejectsBrokenBytecode(t *testing.T) {
	cases := map[string]*bytecode.Bytecode{
		"jump out of range": {Instruction: code.Make(code.OpJump, 9999)},
		"unknown opcode":    {Instruction: []byte{0xfe}},
		"bad const index":   {Instruction: code.Make(code.OpConstant, 5)},
		"stack underflow":   {Instruction: code.Make(code.OpPop)},
		"empty":             {Instruction: nil},
		"nil":               nil,
	}
	for name, bc := range cases {
		if err := bytecode.Validate(bc); err == nil {
			t.Fatalf("%s: should be rejected", name)
		}
	}

	// 方法下标越界：没有方法名表却调用方法
	ins := code.Make(code.OpGetLocal, 0)
	ins = append(ins, code.Make(code.OpCallMethod, 3, 0)...)
	ins = append(ins, code.Make(code.OpReturnVal)...)
	if err := bytecode.Validate(&bytecode.Bytecode{Instruction: ins}); err == nil {
		t.Fatal("method index out of range should be rejected")
	}
}

// 编码后再解码，指令流必须逐字节相同。
func TestCachePreservesInstructions(t *testing.T) {
	orig := compileSample(t, true)
	blob, err := bytecode.Encode(orig, bytecode.CacheMeta{SourceHash: bytecode.SourceHash([]byte(sampleSrc))})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	bc, _, err := bytecode.Decode(blob)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(bc.Instruction) != len(orig.Instruction) {
		t.Fatalf("instruction length: want %d, got %d", len(orig.Instruction), len(bc.Instruction))
	}
	for i := range orig.Instruction {
		if bc.Instruction[i] != orig.Instruction[i] {
			t.Fatalf("instruction byte %d differs (%d != %d)", i, bc.Instruction[i], orig.Instruction[i])
		}
	}
	if bc.MainLocals != orig.MainLocals || bc.MainMaxStack != orig.MainMaxStack {
		t.Fatalf("frame info lost: locals %d/%d, maxstack %d/%d",
			bc.MainLocals, orig.MainLocals, bc.MainMaxStack, orig.MainMaxStack)
	}
}
