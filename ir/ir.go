// Package ir 是字节码与优化之间的中间表示（三地址码 / SSA）。
//
// 为什么需要它：栈式字节码里"表达式结果"没有名字，只有栈位置，
// 而 SSA 的前提是"每个值有唯一的名字和唯一的定义点"。
// 业界做法（HotSpot C2、V8 TurboFan、LLVM 都是这个结构）是：
//
//	AST ─► 栈字节码 ─► IR（三地址码）─► CFG ─► SSA ─► 优化 ─► 降回栈字节码
//
// 把字节码提升成 IR 的过程叫 stack-to-register promotion：
// 用一个抽象栈模拟字节码，每次 push 分配一个新的虚拟值（ValueID）。
// 局部变量本来就在槽位里，因此它们不需要虚拟化，直接作为"变量"参与 SSA。
//
// 本包对外的唯一入口是 Optimize，遇到不支持的指令形态会整体放弃优化
// （返回原始字节码），保证"要么正确优化，要么不优化"。
package ir

import (
	"fmt"

	"Interpreter/object"
)

// ValueID 是虚拟值编号；0 表示"无值"。
type ValueID int32

// NoValue 是无效值编号。
const NoValue ValueID = 0

// Op 是 IR 指令的操作码。
type Op uint8

const (
	// OpConst 把常量池第 Imm 项装入 Dst。
	OpConst Op = iota
	// OpInit 表示"函数入口时局部槽位 Imm 的值"（即形参）。
	// 它没有对应的字节码：值本来就在那个槽位里。
	OpInit
	// OpLoadLocal 读取局部槽位 Imm 的当前值（SSA 阶段会被替换成对应版本）。
	OpLoadLocal
	// OpStoreLocal 把 Args[0] 写入局部槽位 Imm（SSA 之后会被消掉）。
	OpStoreLocal
	// OpBin 是二元运算，Imm 是 code.Opcode（OpAdd/OpGT/...）。
	OpBin
	// OpUn 是一元运算，Imm 是 code.Opcode（OpMinus/OpNot/OpPlus）。
	OpUn
	// OpBuiltin 取第 Imm 个内建函数。
	OpBuiltin
	// OpMakeArray 用 Args 构造数组。
	OpMakeArray
	// OpMakeMap 用 Args（交替的 key/value）构造 map。
	OpMakeMap
	// OpIndex 计算 Args[0][Args[1]]。
	OpIndex
	// OpUpdate 执行 Args[1][Args[2]] = Args[0]（原地修改，有副作用）。
	OpUpdate
	// OpMakeClosure 取常量池第 Imm 项（编译后的函数对象）。
	OpMakeClosure
	// OpCall 调用 Args[0]，其余为实参。
	OpCall
	// OpLoadGlobal 读取全局槽位 Imm（调用可能改写全局，故不做 SSA）。
	OpLoadGlobal
	// OpStoreGlobal 把 Args[0] 写入全局槽位 Imm。
	OpStoreGlobal
	// OpReturn 返回 Args[0]。
	OpReturn
	// OpJump 无条件跳转到块 Imm。
	OpJump
	// OpBranch 在 Args[0] 为真时跳到 Imm2，否则跳到 Imm。
	OpBranch
	// OpPhi 是 SSA 的 phi 节点，实参按前驱顺序排列。
	OpPhi
	// OpExit 表示函数在这里自然结束（main 跑完最后一条指令，不返回值）。
	OpExit
	// OpSink 表示"这个值被丢弃了，但丢弃本身是可观察的"（OpPop 会记录 lastPop）。
	OpSink
	// OpNop 是 SSA 构造过程中的占位（稍后被删除）。
	OpNop
)

func (o Op) String() string {
	switch o {
	case OpConst:
		return "const"
	case OpInit:
		return "init"
	case OpStoreLocal:
		return "store.local"
	case OpLoadLocal:
		return "load.local"
	case OpBin:
		return "bin"
	case OpUn:
		return "un"
	case OpBuiltin:
		return "builtin"
	case OpMakeArray:
		return "array"
	case OpMakeMap:
		return "map"
	case OpIndex:
		return "index"
	case OpUpdate:
		return "update"
	case OpMakeClosure:
		return "closure"
	case OpCall:
		return "call"
	case OpLoadGlobal:
		return "load.global"
	case OpStoreGlobal:
		return "store.global"
	case OpReturn:
		return "return"
	case OpJump:
		return "jump"
	case OpBranch:
		return "branch"
	case OpPhi:
		return "phi"
	case OpExit:
		return "exit"
	case OpSink:
		return "sink"
	case OpNop:
		return "nop"
	}
	return fmt.Sprintf("op(%d)", uint8(o))
}

// Instr 是一条三地址指令。
type Instr struct {
	Op   Op
	Dst  ValueID
	Args []ValueID
	// Imm 的含义随 Op 变化：常量下标 / 槽位 / 内建下标 / 二元运算码 /
	// 跳转目标块 ID。
	Imm int
	// Imm2 目前只用于 OpBranch 的"条件为真"目标。
	Imm2 int
	Line int
}

// IsPure 判断指令是否无副作用（可删除、可用于 CSE）。
//
// 注意 OpIndex 是"读堆"，本身没有副作用，但会被 OpUpdate/OpCall 影响，
// 因此 CSE 时必须把后两者当作屏障，不能跨过去复用。
func (in *Instr) IsPure() bool {
	switch in.Op {
	case OpConst, OpInit, OpBin, OpUn, OpBuiltin, OpMakeArray, OpMakeMap,
		OpIndex, OpMakeClosure, OpPhi:
		return true
	}
	return false
}

// CannotTrap 判断指令在没有任何副作用的同时也不会触发运行时错误。
//
// 只有这样的指令才能在结果无人使用时被整条删除：本语言的运行时错误
// （除零、类型不匹配、下标越界）是程序可观察的行为，删掉会改变语义。
func (in *Instr) CannotTrap() bool {
	switch in.Op {
	case OpConst, OpInit, OpPhi, OpNop, OpBuiltin,
		OpMakeArray, OpMakeMap, OpMakeClosure, OpLoadLocal, OpLoadGlobal:
		return true
	}
	return false
}

// IsTerminator 判断指令是否结束基本块。
func (in *Instr) IsTerminator() bool {
	return in.Op == OpJump || in.Op == OpBranch || in.Op == OpReturn || in.Op == OpExit
}

// Block 是 IR 的基本块。
type Block struct {
	ID     int
	Offset int // 对应字节码里的起始偏移，仅用于调试
	Instrs []*Instr
	Preds  []*Block
	Succs  []*Block

	// 活跃性分析结果
	liveIn  map[ValueID]bool
	liveOut map[ValueID]bool
}

// Last 返回块的最后一条指令。
func (b *Block) Last() *Instr {
	if len(b.Instrs) == 0 {
		return nil
	}
	return b.Instrs[len(b.Instrs)-1]
}

// Function 是一个函数的 IR。
type Function struct {
	Name   string
	Params int
	Blocks []*Block
	Entry  *Block

	pool     *constPool
	numValue ValueID
	defs     []*Instr
	types    []valueType
	// slot 是寄存器分配结果：每个值落在哪个局部槽位
	slot []int

	// hasCall 表示函数里有调用（调用会改写全局与堆，是优化屏障）
	hasCall bool
	// ssaGlobals 表示本次对全局槽位也做 SSA（仅当 main 内没有任何调用）
	ssaGlobals bool
	// dropGlobalStores 表示可以彻底删除写全局的指令（全局已成私有变量）
	dropGlobalStores bool
	// isMain 标记这是顶层函数
	isMain bool
	// noSSAGlobals 记录不参与 SSA 的全局槽位（需要 phi 的那些）
	noSSAGlobals map[int]bool
}

// ValueCount 返回已分配的值数量（含 0 号占位）。
func (f *Function) ValueCount() int { return int(f.numValue) }

// Def 返回定义该值的指令（可能为 nil）。
func (f *Function) Def(v ValueID) *Instr {
	if v <= 0 || int(v) >= len(f.defs) {
		return nil
	}
	return f.defs[v]
}

// Slot 返回寄存器分配后该值所在的局部槽位。
func (f *Function) Slot(v ValueID) int {
	if v <= 0 || int(v) >= len(f.slot) {
		return -1
	}
	return f.slot[v]
}

// Const 返回常量池第 i 项。
func (f *Function) Const(i int) object.Object { return f.pool.objs[i] }

// NewValue 分配一个新的虚拟值。
// defs/types 以 ValueID 为下标，因此长度始终是 numValue+1（0 号占位）。
func (f *Function) NewValue() ValueID {
	f.numValue++
	for int(f.numValue) >= len(f.defs) {
		f.defs = append(f.defs, nil)
		f.types = append(f.types, typeUnknown)
	}
	return f.numValue
}

// Emit 追加一条指令；Dst 非 0 时登记为它的定义点。
func (f *Function) Emit(b *Block, in *Instr) *Instr {
	if in.Dst != NoValue {
		for int(in.Dst) >= len(f.defs) {
			f.defs = append(f.defs, nil)
			f.types = append(f.types, typeUnknown)
		}
		f.defs[in.Dst] = in
	}
	b.Instrs = append(b.Instrs, in)
	return in
}

// ---------------------------------------------------------------------------
// 常量池
// ---------------------------------------------------------------------------

// constPool 是整份字节码共享的常量池。优化过程中可能新增常量
// （例如常量折叠产生的 7），因此需要去重后追加。
type constPool struct {
	objs []object.Object
	idx  map[string]int
}

func newConstPool(objs []object.Object) *constPool {
	p := &constPool{
		objs: make([]object.Object, len(objs)),
		idx:  make(map[string]int, len(objs)),
	}
	copy(p.objs, objs)
	for i, o := range objs {
		p.idx[constKey(o)] = i
	}
	return p
}

func constKey(o object.Object) string {
	return string(o.Type()) + "\x00" + o.Inspect()
}

func (p *constPool) intern(o object.Object) int {
	k := constKey(o)
	if i, ok := p.idx[k]; ok {
		return i
	}
	p.objs = append(p.objs, o)
	i := len(p.objs) - 1
	p.idx[k] = i
	return i
}

// ---------------------------------------------------------------------------
// 值的类型（仅用于安全地做代数化简与折叠）
// ---------------------------------------------------------------------------

type valueType uint8

const (
	typeUnknown valueType = iota
	typeInt
	typeFloat
	typeString
	typeBool
	typeNull
)

func typeOfObject(o object.Object) valueType {
	switch o.(type) {
	case object.Int:
		return typeInt
	case object.Float:
		return typeFloat
	case object.String:
		return typeString
	case object.Boolean:
		return typeBool
	case object.Null:
		return typeNull
	}
	return typeUnknown
}

func isNumeric(t valueType) bool { return t == typeInt || t == typeFloat }

func (t valueType) String() string {
	switch t {
	case typeInt:
		return "int"
	case typeFloat:
		return "float"
	case typeString:
		return "string"
	case typeBool:
		return "bool"
	case typeNull:
		return "null"
	}
	return "unknown"
}

// ---------------------------------------------------------------------------
// 放弃优化
// ---------------------------------------------------------------------------

// unsupported 表示遇到本优化器还不支持的指令形态。
// 捕获它即可安全回退到未优化的字节码。
type unsupported struct{ why string }

func (u unsupported) Error() string { return "ir: unsupported: " + u.why }

func bail(format string, args ...interface{}) error {
	return unsupported{why: fmt.Sprintf(format, args...)}
}
