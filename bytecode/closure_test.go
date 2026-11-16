package bytecode_test

import (
	"bytes"
	"strings"
	"testing"

	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/object"
)

func TestCacheRejectsInvalidClosureMetadata(t *testing.T) {
	tests := []struct {
		name string
		op   code.Opcode
		free []object.FreeVar
		body code.Instructions
		want string
	}{
		{"local capture out of bounds", code.OpCaptureClosure,
			[]object.FreeVar{{Local: true, Index: 1}},
			code.Make(code.OpNull), "capture index"},
		{"inherited capture without enclosing closure", code.OpCaptureClosure,
			[]object.FreeVar{{Index: 0}},
			code.Make(code.OpNull), "capture index"},
		{"negative capture", code.OpCaptureClosure,
			[]object.FreeVar{{Local: true, Index: -1}},
			code.Make(code.OpNull), "capture index"},
		{"load missing free variable", code.OpCaptureClosure,
			[]object.FreeVar{{Local: true, Index: 0}},
			code.Make(code.OpGetFree, 1), "free variable index"},
		{"store missing free variable", code.OpCaptureClosure,
			[]object.FreeVar{{Local: true, Index: 0}},
			append(code.Make(code.OpNull), code.Make(code.OpSetFree, 1)...), "free variable index"},
		{"bare function needs environment", code.OpClosure,
			[]object.FreeVar{{Local: true, Index: 0}},
			code.Make(code.OpNull), "requires a closure environment"},
		{"constant call needs environment", code.OpCallFuncConst,
			[]object.FreeVar{{Local: true, Index: 0}},
			code.Make(code.OpNull), "requires a closure environment"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			operands := []int{0}
			if tc.op == code.OpCallFuncConst {
				operands = append(operands, 0)
			}
			bc := &bytecode.Bytecode{
				MainLocals:  1,
				Instruction: code.Make(tc.op, operands...),
				Constants: []object.Object{object.CompiledFunc{
					FnName:       "captured_fn",
					FreeVars:     tc.free,
					Instructions: tc.body,
				}},
			}
			if err := bytecode.Validate(bc); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			blob, err := bytecode.Encode(bc, bytecode.CacheMeta{})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := bytecode.Decode(blob); err == nil {
				t.Fatal("invalid capture metadata accepted from cache")
			}
		})
	}
}

func TestCacheRejectsTruncatedClosureOperands(t *testing.T) {
	for _, op := range []code.Opcode{code.OpCaptureClosure, code.OpGetFree, code.OpSetFree} {
		for _, ins := range []code.Instructions{{byte(op)}, {byte(op), 0}} {
			bc := &bytecode.Bytecode{Instruction: ins}
			if err := bytecode.Validate(bc); err == nil {
				t.Fatalf("accepted truncated instruction %v", ins)
			}
		}
	}
}

func TestCacheRejectsInvalidCaptureKind(t *testing.T) {
	const name = "unique_captured_function"
	bc := &bytecode.Bytecode{
		Instruction: code.Make(code.OpCaptureClosure, 0),
		MainLocals:  1,
		Constants: []object.Object{object.CompiledFunc{
			FnName:       name,
			FreeVars:     []object.FreeVar{{Local: true, Index: 0}},
			Instructions: code.Make(code.OpGetFree, 0),
		}},
	}
	blob, err := bytecode.Encode(bc, bytecode.CacheMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := bytecode.Decode(blob); err != nil {
		t.Fatalf("valid capture rejected: %v", err)
	}
	start := bytes.Index(blob, []byte(name))
	if start < 0 {
		t.Fatal("function name missing from cache")
	}
	// Five function fields and the capture count precede the first capture kind.
	blob[start+len(name)+6*4] = 2
	if _, _, err := bytecode.Decode(blob); err == nil || !strings.Contains(err.Error(), "invalid capture kind") {
		t.Fatalf("invalid capture kind accepted: %v", err)
	}
}
