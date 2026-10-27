package vm

import (
	"testing"

	"Interpreter/object"
)

// 帧的局部槽位来自共享槽位区：返回后必须能原样复用。
func TestAllocLocalsReusesReleasedSlots(t *testing.T) {
	vm := NewVM()
	vm.frames[0] = Frame{ip: -1}

	outer := vm.allocLocals(3)
	for i := range outer {
		outer[i] = object.Int{Value: i}
	}
	if vm.localsTop != 3 {
		t.Fatalf("localsTop = %d, want 3", vm.localsTop)
	}

	inner := vm.allocLocals(2)
	if len(inner) != 2 {
		t.Fatalf("inner len = %d, want 2", len(inner))
	}
	for i := range inner {
		inner[i] = NullObj
	}

	// 模拟内层帧返回
	vm.localsTop = 3
	again := vm.allocLocals(2)
	if &again[0] != &inner[0] {
		t.Fatal("released slots should be handed out again from the same arena")
	}
}

// 槽位区扩容时，已存在的帧必须继续指向搬迁后的数组。
func TestAllocLocalsGrowthKeepsExistingFramesValid(t *testing.T) {
	vm := NewVM()
	vm.locals = make([]object.Object, 2)
	vm.frames[0] = Frame{ip: -1}
	first := vm.allocLocals(2)
	vm.frames[0].vars = first
	vm.frames[0].localsBase = 0
	vm.frameIdx = 1
	sentinel := object.Int{Value: 42}
	first[1] = sentinel

	// 这次分配会触发扩容
	second := vm.allocLocals(8)
	if len(second) != 8 {
		t.Fatalf("second len = %d, want 8", len(second))
	}
	if len(vm.locals) <= 2 {
		t.Fatal("locals arena should have grown")
	}
	got := vm.frames[0].vars[1]
	if got != sentinel {
		t.Fatalf("existing frame lost its value after growth: %v", got)
	}
}

// 新分配的槽位必须是 None，而不是 nil（nil 会在后续类型判断时崩溃）。
func TestAllocLocalsZeroesToNull(t *testing.T) {
	vm := NewVM()
	vm.frames[0] = Frame{ip: -1}
	vars := vm.allocLocals(4)
	for i, v := range vars {
		if v == nil {
			t.Fatalf("slot %d is nil", i)
		}
		if _, ok := v.(object.Null); !ok {
			t.Fatalf("slot %d = %v, want None", i, v)
		}
	}
}
