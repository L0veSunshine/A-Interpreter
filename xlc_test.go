package main

import (
	"bytes"
	"testing"

	"Interpreter/bytecode"
	"Interpreter/vm"
)

// 缓存是"跳过前端"的捷径，所以最强的检查是：**从缓存载入后执行的结果，
// 必须和她直接从源码编译执行完全一致**。这里用整个算法语料对拍。
func TestCacheProducesIdenticalResults(t *testing.T) {
	for _, prog := range algorithmPrograms {
		t.Run(prog.name, func(t *testing.T) {
			src := algorithmSource(t, prog.name)
			for _, optimize := range []bool{true, false} {
				direct := compileAlgorithm(t, src, optimize)
				meta := bytecode.CacheMeta{
					Optimized:   optimize,
					SourceHash:  bytecode.SourceHash([]byte(src)),
					SourceSize:  int64(len(src)),
					SourceMTime: 1700000000000000000,
				}
				blob, err := bytecode.Encode(direct, meta)
				if err != nil {
					t.Fatalf("optimize=%v: encode: %v", optimize, err)
				}
				loaded, gotMeta, err := bytecode.Decode(blob)
				if err != nil {
					t.Fatalf("optimize=%v: decode: %v", optimize, err)
				}
				if gotMeta != meta {
					t.Fatalf("optimize=%v: meta roundtrip mismatch", optimize)
				}

				// 直接跑 vs 跑缓存
				want := runAlgorithm(t, direct)
				m := vm.NewVM()
				if err := m.Run(loaded); err != nil {
					t.Fatalf("optimize=%v: run from cache: %v", optimize, err)
				}
				if got := m.LastPop().Inspect(); got != want {
					t.Fatalf("optimize=%v: cache result %s != direct %s", optimize, got, want)
				}
				if want != prog.want {
					t.Fatalf("optimize=%v: want %s, got %s", optimize, prog.want, want)
				}
			}
		})
	}
}

// 同一份源码重复编码必须得到完全相同的字节（缓存可复现、可 diff）。
func TestCacheEncodingIsDeterministic(t *testing.T) {
	src := algorithmSource(t, "fib")
	bc := compileAlgorithm(t, src, true)
	meta := bytecode.CacheMeta{Optimized: true, SourceHash: bytecode.SourceHash([]byte(src))}
	first, err := bytecode.Encode(bc, meta)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	second, err := bytecode.Encode(bc, meta)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("encoding is not deterministic")
	}
}

// 运行期错误（行号）也必须能从缓存里复现：行号表要完整保存。
func TestCacheKeepsRuntimeErrorLine(t *testing.T) {
	src := `var a = 1
var b = 0
var c = a / b`
	bc := compileAlgorithm(t, src, true)
	blob, err := bytecode.Encode(bc, bytecode.CacheMeta{SourceHash: bytecode.SourceHash([]byte(src))})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	loaded, _, err := bytecode.Decode(blob)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	m := vm.NewVM()
	err = m.Run(loaded)
	if err == nil {
		t.Fatal("expected division by zero")
	}
	if msg := err.Error(); !bytes.Contains([]byte(msg), []byte("line 3")) {
		t.Fatalf("error should point at line 3, got %q", msg)
	}
}
