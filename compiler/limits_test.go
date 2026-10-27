package compiler

import (
	"Interpreter/code"
	"Interpreter/parser"
	"strings"
	"testing"
)

func newTestCompiler() *Compiler {
	c := NewCompiler()
	c.SetSymbol(parser.NewSymTable("Base"))
	return c
}

// 操作数放不进声明的位宽时必须报编译错误，而不是被静默截断。
func TestEmitRejectsOperandOverflow(t *testing.T) {
	cases := []struct {
		name  string
		op    code.Opcode
		arg   int
		limit int
	}{
		{"global slot", code.OpGetGlobal, 65536, 65535},
		{"local slot", code.OpSetLocal, 1 << 16, 65535},
		{"constant index", code.OpConstant, 70000, 65535},
		{"builtin index", code.OpGetBuiltin, 256, 255},
		{"arg count", code.OpCallFunc, 256, 255},
		{"negative", code.OpGetGlobal, -1, 65535},
	}
	for _, tc := range cases {
		c := newTestCompiler()
		c.emit(tc.op, tc.arg)
		if !c.HasError() {
			t.Fatalf("%s: operand %d should be rejected (limit %d)", tc.name, tc.arg, tc.limit)
		}
		if !strings.Contains(c.Errs()[0].Error(), "exceeds") {
			t.Fatalf("%s: unexpected error %v", tc.name, c.Errs()[0])
		}
	}
}

func TestEmitAcceptsOperandAtLimit(t *testing.T) {
	c := newTestCompiler()
	c.emit(code.OpGetGlobal, 65535)
	c.emit(code.OpGetBuiltin, 255)
	c.emit(code.OpConstant, 65535)
	if c.HasError() {
		t.Fatalf("operands at the limit must be accepted, got %v", c.Errs())
	}
	if got := len(c.curInstruction()); got != 8 {
		t.Fatalf("expected 8 bytes emitted (3+2+3), got %d", got)
	}
}

// 跳转目标同样受 2 字节限制：函数体超过 64KB 时要报错而不是跳错位置。
func TestChangeOperandRejectsOverflow(t *testing.T) {
	c := newTestCompiler()
	pos := c.emit(code.OpJump, 0)
	c.changeOperand(pos, 65536)
	if !c.HasError() {
		t.Fatal("jump target beyond 65535 should be rejected")
	}
}

// 上限内的跳转回填不能误报。
func TestChangeOperandAcceptsInRange(t *testing.T) {
	c := newTestCompiler()
	pos := c.emit(code.OpJump, 9999)
	c.changeOperand(pos, 65535)
	if c.HasError() {
		t.Fatalf("in-range jump target rejected: %v", c.Errs())
	}
}
