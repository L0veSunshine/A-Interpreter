package ir

import (
	"testing"

	"Interpreter/code"
	"Interpreter/object"
)

func TestBuildMethodCallDualResults(t *testing.T) {
	var ins code.Instructions
	for _, op := range []code.Instructions{
		code1(code.OpBuildArray, 0),
		code1(code.OpConstant, 0),
		code1(code.OpConstant, 1),
		code1(code.OpCallMethod, 7, 2),
		code1(code.OpSetLocal, 0),
		code1(code.OpPop),
	} {
		ins = append(ins, op...)
	}
	fn, err := buildFunction("method", 0, ins, nil,
		testPool(object.Int{Value: 1}, object.Int{Value: 2}), false)
	if err != nil {
		t.Fatal(err)
	}
	call := fn.Entry.Instrs[3]
	if call.Op != OpMethodCall || call.Imm != 7 || len(call.Args) != 3 {
		t.Fatalf("malformed method IR: %+v", call)
	}
	if call.Dst == NoValue || call.Dst2 == NoValue || call.Dst == call.Dst2 {
		t.Fatalf("method must define distinct results: %+v", call)
	}
	for i, arg := range call.Args {
		if arg != fn.Entry.Instrs[i].Dst {
			t.Fatalf("argument %d changed evaluation order", i)
		}
	}
	if fn.Def(call.Dst) != call || fn.Def(call.Dst2) != call {
		t.Fatal("both results must have registered definition points")
	}
	if fn.Entry.Instrs[4].Args[0] != call.Dst2 || fn.Entry.Instrs[5].Args[0] != call.Dst {
		t.Fatal("builder reversed result/self stack order")
	}
	if !fn.hasCall {
		t.Fatal("method call must prevent unsafe global SSA")
	}
	if err := fn.runSSA(); err != nil {
		t.Fatal(err)
	}
	if err := fn.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildMethodCallRejectsUnderflow(t *testing.T) {
	for _, tc := range []struct {
		name string
		ins  code.Instructions
	}{
		{"missing receiver", code1(code.OpCallMethod, 0, 0)},
		{"missing argument", append(code1(code.OpBuildArray, 0), code1(code.OpCallMethod, 0, 1)...)},
		{"missing two arguments", append(code1(code.OpBuildArray, 0), code1(code.OpCallMethod, 0, 2)...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildFunction("bad", 0, tc.ins, nil, testPool(), false); err == nil {
				t.Fatal("expected operand stack underflow")
			}
		})
	}
}

func TestMethodCallResultsLivenessAndAllocation(t *testing.T) {
	fn := &Function{Name: "method", Params: 1, pool: testPool()}
	entry, next := &Block{ID: 0}, &Block{ID: 1}
	fn.Entry, fn.Blocks = entry, []*Block{entry, next}
	param, self, result, newSelf := fn.NewValue(), fn.NewValue(), fn.NewValue(), fn.NewValue()
	fn.Emit(entry, &Instr{Op: OpInit, Dst: param, Imm: 0})
	fn.Emit(entry, &Instr{Op: OpMakeArray, Dst: self})
	fn.Emit(entry, &Instr{Op: OpMethodCall, Dst: result, Dst2: newSelf, Args: []ValueID{self}})
	fn.Emit(entry, &Instr{Op: OpJump, Imm: 1})
	for _, v := range []ValueID{result, newSelf, param} {
		fn.Emit(next, &Instr{Op: OpSink, Args: []ValueID{v}})
	}
	fn.Emit(next, &Instr{Op: OpExit})
	fn.linkBlocks()
	fn.allocateSlots()
	for _, v := range []ValueID{result, newSelf} {
		if !entry.liveOut[v] || !next.liveIn[v] {
			t.Fatalf("method result %d must be live across edge", v)
		}
		if entry.liveIn[v] {
			t.Fatalf("method result %d must be killed by its definition", v)
		}
	}
	for _, pair := range [][2]ValueID{{result, newSelf}, {result, param}, {newSelf, param}} {
		if fn.Slot(pair[0]) < 0 || fn.Slot(pair[0]) == fn.Slot(pair[1]) {
			t.Fatalf("simultaneously live values %v share slot %d", pair, fn.Slot(pair[0]))
		}
	}
	if fn.Slot(param) != 0 {
		t.Fatal("parameter must retain its ABI slot")
	}
	defBlocks := fn.defBlocks()
	if defBlocks[result] != entry || defBlocks[newSelf] != entry {
		t.Fatal("both method results must have a defining block")
	}
	cross := fn.crossBlockUses(defBlocks)
	if !cross[result] || !cross[newSelf] {
		t.Fatal("both results must be materialized for cross-block uses")
	}
}

func TestMethodCallIsCSEBarrier(t *testing.T) {
	fn := &Function{Name: "method", pool: testPool(object.Int{Value: 0})}
	b := &Block{ID: 0}
	fn.Entry, fn.Blocks = b, []*Block{b}
	self, key := fn.NewValue(), fn.NewValue()
	fn.Emit(b, &Instr{Op: OpMakeArray, Dst: self})
	fn.Emit(b, &Instr{Op: OpConst, Dst: key})
	before, duplicate := fn.NewValue(), fn.NewValue()
	fn.Emit(b, &Instr{Op: OpIndex, Dst: before, Args: []ValueID{self, key}})
	fn.Emit(b, &Instr{Op: OpIndex, Dst: duplicate, Args: []ValueID{self, key}})
	call := &Instr{Op: OpMethodCall, Dst: fn.NewValue(), Dst2: fn.NewValue(), Args: []ValueID{self}}
	fn.Emit(b, call)
	after := fn.NewValue()
	fn.Emit(b, &Instr{Op: OpIndex, Dst: after, Args: []ValueID{self, key}})
	sink := fn.Emit(b, &Instr{Op: OpMakeArray, Dst: fn.NewValue(), Args: []ValueID{before, duplicate, after}})
	fn.cse()
	if sink.Args[0] != sink.Args[1] {
		t.Fatal("identical reads before the call should be reused")
	}
	if sink.Args[0] == sink.Args[2] {
		t.Fatal("heap read must not be reused across method mutation")
	}
	if call.IsPure() || call.CannotTrap() {
		t.Fatal("method calls can mutate and raise errors")
	}
}

func TestMethodCallDCEKeepsUnusedResults(t *testing.T) {
	for _, keep := range []ValueID{NoValue, 1, 2} {
		fn := &Function{Name: "method", pool: testPool()}
		b := &Block{ID: 0}
		fn.Entry, fn.Blocks = b, []*Block{b}
		result, self := fn.NewValue(), fn.NewValue()
		receiver := fn.NewValue()
		makeReceiver := fn.Emit(b, &Instr{Op: OpMakeArray, Dst: receiver})
		call := fn.Emit(b, &Instr{Op: OpMethodCall, Dst: result, Dst2: self, Args: []ValueID{receiver}})
		if keep != NoValue {
			fn.Emit(b, &Instr{Op: OpSink, Args: []ValueID{keep}})
		}
		fn.Emit(b, &Instr{Op: OpExit})
		fn.removeDeadCode()
		found, foundReceiver := false, false
		for _, in := range b.Instrs {
			found = found || in == call
			foundReceiver = foundReceiver || in == makeReceiver
		}
		if !found || !foundReceiver {
			t.Fatalf("method call with retained result %d was deleted", keep)
		}
	}
}

func TestLowerUnusedMethodResultsDoesNotEmitPop(t *testing.T) {
	fn := &Function{Name: "method", pool: testPool()}
	b := &Block{ID: 0}
	fn.Entry, fn.Blocks = b, []*Block{b}
	receiver, result, self := fn.NewValue(), fn.NewValue(), fn.NewValue()
	fn.Emit(b, &Instr{Op: OpMakeArray, Dst: receiver})
	fn.Emit(b, &Instr{Op: OpMethodCall, Dst: result, Dst2: self, Args: []ValueID{receiver}})
	fn.Emit(b, &Instr{Op: OpExit})
	fn.linkBlocks()
	ins, _, _, err := fn.lower(true)
	if err != nil {
		t.Fatal(err)
	}
	calls, stores := 0, 0
	for pos := 0; pos < len(ins); {
		op := code.Opcode(ins[pos])
		def, ok := code.Definitions[op]
		if !ok {
			t.Fatalf("unknown opcode %d", op)
		}
		switch op {
		case code.OpPop:
			t.Fatal("discarding internal results via OpPop changes observable lastPop")
		case code.OpCallMethod:
			calls++
		case code.OpSetLocal:
			stores++
		}
		pos++
		for _, width := range def.OperandWidth {
			pos += width
		}
	}
	if calls != 1 || stores != 2 {
		t.Fatalf("dual-result call must be emitted once and store both results: calls=%d stores=%d", calls, stores)
	}
}
