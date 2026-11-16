package vm_test

import (
	"testing"

	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/ir"
	"Interpreter/vm"
)

// compileFusion 按指定的融合开关编译源码。
func compileFusion(t testing.TB, src string, fusion bool) *bytecode.Bytecode {
	t.Helper()
	old := ir.FusionEnabled
	ir.FusionEnabled = fusion
	defer func() { ir.FusionEnabled = old }()
	return compile(t, src)
}

// runResult 执行字节码，返回最后一条语句的值或错误文本。
func runResult(bc *bytecode.Bytecode) (string, string) {
	m := vm.NewVM()
	if err := m.Run(bc); err != nil {
		return "", err.Error()
	}
	return m.LastPop().Inspect(), ""
}

// 下标融合必须与未融合时逐字一致：结果、错误信息都相同。
//
// 下标通过函数参数传入：参数是运行时局部变量，编译器无法做常量折叠，
// 因此 `i+1` 等算术会真实保留在字节码里。顶层用例则用循环改写的全局变量，
// 同样不可折叠，用来覆盖全局操作数的编码路径。
// fused 非 0 的用例还要求融合后的字节码里确实出现对应指令。
func TestKeyedBinFusedEquivalence(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		want    string
		wantErr bool
		fused   code.Opcode
	}{
		{
			name:  "read array with add key",
			src:   "def get(a, i){\n\treturn a[i+1]\n}\nget([1,2,3,4,5], 2)",
			want:  "4",
			fused: code.OpIndexBinFused,
		},
		{
			name:  "read array with sub key",
			src:   "def get(a, i){\n\treturn a[i-1]\n}\nget([1,2,3,4,5], 2)",
			want:  "2",
			fused: code.OpIndexBinFused,
		},
		{
			name:  "negative key wraps",
			src:   "def get(a, i){\n\treturn a[i-1]\n}\nget([1,2,3], 0)",
			want:  "3",
			fused: code.OpIndexBinFused,
		},
		{
			name:  "mul key",
			src:   "def get(a, i){\n\treturn a[i*2]\n}\nget([1,2,3], 1)",
			want:  "3",
			fused: code.OpIndexBinFused,
		},
		{
			name:    "read out of range",
			src:     "def get(a, i){\n\treturn a[i+5]\n}\nget([1,2,3], 0)",
			wantErr: true,
			fused:   code.OpIndexBinFused,
		},
		{
			name:    "mod by zero key",
			src:     "def get(a, i){\n\treturn a[i%i]\n}\nget([1,2,3], 0)",
			wantErr: true,
			fused:   code.OpIndexBinFused,
		},
		{
			name:    "div key yields float and is rejected like unfused",
			src:     "def get(a, i){\n\treturn a[i/2]\n}\nget([1,2,3], 4)",
			wantErr: true,
			fused:   code.OpIndexBinFused,
		},
		{
			name:  "read string with computed key",
			src:   "def get(s, i){\n\treturn s[i+1]\n}\nget(\"hello\", 2)",
			want:  "l",
			fused: code.OpIndexBinFused,
		},
		{
			name:  "read map with computed int key",
			src:   "def get(m, i){\n\treturn m[i+1]\n}\nget({1: 10}, 0)",
			want:  "10",
			fused: code.OpIndexBinFused,
		},
		{
			name:  "write array with add key",
			src:   "def set(a, i){\n\ta[i+1]=9\n\treturn a\n}\nset([1,2,3], 0)",
			want:  "[1, 9, 3]",
			fused: code.OpUpdateBinFused,
		},
		{
			name:  "write array with negative key",
			src:   "def set(a, i){\n\ta[i-3]=7\n\treturn a\n}\nset([1,2,3], 2)",
			want:  "[1, 2, 7]",
			fused: code.OpUpdateBinFused,
		},
		{
			name:    "write out of range",
			src:     "def set(a, i){\n\ta[i+10]=1\n\treturn a\n}\nset([1,2,3], 2)",
			wantErr: true,
			fused:   code.OpUpdateBinFused,
		},
		{
			name:  "write map with computed int key",
			src:   "def set(m, i){\n\tm[i+1]=5\n\treturn m[i+1]\n}\nset({}, 0)",
			want:  "5",
			fused: code.OpUpdateBinFused,
		},
		{
			name:  "global index mutated in loop",
			src:   "var a=[1,2,3,4,5]\nvar i=0\nfor(var j=0;j<2;j=j+1){\n\ti=i+1\n}\na[i+1]",
			want:  "4",
			fused: code.OpIndexBinFused,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fusedBC := compileFusion(t, tc.src, true)
			plainBC := compileFusion(t, tc.src, false)

			gotFused, errFused := runResult(fusedBC)
			gotPlain, errPlain := runResult(plainBC)

			if gotFused != gotPlain || errFused != errPlain {
				t.Fatalf("fused and unfused differ:\n fused: %q / %q\n plain: %q / %q",
					gotFused, errFused, gotPlain, errPlain)
			}
			if tc.wantErr {
				if errFused == "" {
					t.Fatalf("expected an error, got result %q", gotFused)
				}
			} else if errFused != "" {
				t.Fatalf("unexpected error: %s", errFused)
			} else if gotFused != tc.want {
				t.Fatalf("got %q, want %q", gotFused, tc.want)
			}

			if tc.fused != 0 {
				if n := countAll(fusedBC, tc.fused); n == 0 {
					t.Fatalf("expected %s in fused bytecode", code.Definitions[tc.fused].Name)
				}
				if n := countAll(plainBC, tc.fused); n != 0 {
					t.Fatalf("fusion disabled but found %s", code.Definitions[tc.fused].Name)
				}
			}
		})
	}
}
