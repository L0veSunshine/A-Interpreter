package bytecode

// 本文件实现 `.xlc` 缓存：把编译产物（栈字节码 + 常量池 + 行号表）序列化到磁盘，
// 下次运行同一个源文件时直接加载，跳过词法/解析/编译/优化。
//
// 设计要点（对照 CPython 的 `.pyc`）：
//
//  1. 文件头带 **magic + 格式版本**：解释器升级后旧缓存一律失效，绝不半信半疑地读；
//  2. 带 **源文件指纹**（SHA-256）：源改了但 mtime 没变也能发现（mtime 只作为快速路径）；
//  3. 载入是**不可信输入**：逐字段做边界检查，最后再跑一遍 code.MaxStack 校验
//     指令流（未知操作码、越界跳转、栈下溢、汇合深度不一致都会被拒），
//     任何异常都当成"没有缓存"重新编译，而不是把坏字节码交给 VM；
//  4. 只存"执行必需"的符号信息：方法名表（OpCallMethod 的操作数要用它查实现）。
//     局部变量名只影响反汇编美观度，不存。

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"Interpreter/code"
	"Interpreter/object"
	"Interpreter/parser"
)

// CacheMagic 是文件头，用于识别"这确实是一个 xlc 文件"。
const CacheMagic = "XLC1"

// CacheFormatVersion 必须与操作码集合、Bytecode 结构、VM 语义保持同步。
// 任何一项变化都要 +1 —— 旧缓存会被自动丢弃（不会读到不兼容的字节码）。
const CacheFormatVersion uint16 = 3

// 载入时对各个计数的上限，避免一个畸形文件让我们去申请几个 G 的内存。
const (
	maxInstructions = 64 << 20 // 字节
	maxConstants    = 1 << 20
	maxFuncs        = 1 << 16
	maxLines        = 1 << 22
	maxMethods      = 1 << 16
	maxStringBytes  = 1 << 24
	maxNameBytes    = 1 << 12
)

// 常量池里的对象标签。
const (
	tagNull uint8 = iota
	tagInt
	tagFloat
	tagBool
	tagString
	tagFunc
	tagArray
)

// ErrNotCache 表示"这个文件不是可用的 xlc 缓存"，调用方应当回退到源码编译。
var ErrNotCache = errors.New("xlc: not a usable cache")

// CacheMeta 是缓存里除字节码之外的信息：用来判断"还能不能用"。
type CacheMeta struct {
	// Optimized 记录生成缓存时是否启用了 IR 优化。
	Optimized bool
	// SourceHash 是源文件内容的 SHA-256。
	SourceHash [32]byte
	// SourceSize / SourceMTime 是快速路径：不一致时才去算哈希。
	SourceSize  int64
	SourceMTime int64
}

// 文件头布局（固定偏移，便于外部工具识别）：
//
//	0..3   magic "XLC1"
//	4..5   格式版本 u16
//	6      flags（bit0 = 已优化）
//	7..38  源文件 SHA-256
//	39..46 源文件大小 i64
//	47..54 源文件 mtime（UnixNano）i64
const (
	headerOffsetHash    = 7
	headerOffsetSize    = headerOffsetHash + 32
	headerOffsetMTime   = headerOffsetSize + 8
	cacheHeaderMinBytes = headerOffsetMTime + 8
)

// SourceHash 计算源文件指纹。
func SourceHash(src []byte) [32]byte {
	return sha256.Sum256(src)
}

// Encode 把字节码编码成 .xlc 的字节序列。
func Encode(bc *Bytecode, meta CacheMeta) ([]byte, error) {
	if bc == nil {
		return nil, errors.New("xlc: nil bytecode")
	}
	w := &writer{buf: bytes.NewBuffer(make([]byte, 0, len(bc.Instruction)+512))}
	w.blob([]byte(CacheMagic))
	w.u16(CacheFormatVersion)
	var flags uint8
	if meta.Optimized {
		flags |= 1
	}
	w.u8(flags)
	w.bytes(meta.SourceHash[:])
	w.i64(meta.SourceSize)
	w.i64(meta.SourceMTime)

	// 方法名表：OpCallMethod 的操作数是它的下标。
	methods := methodNames(bc.Symbols)
	w.u32(uint32(len(methods)))
	for _, name := range methods {
		w.str(name)
	}

	w.u32(uint32(len(bc.Constants)))
	for _, c := range bc.Constants {
		if err := w.obj(c); err != nil {
			return nil, err
		}
	}

	w.ins(bc.Instruction)
	w.lines(bc.Lines)
	w.u32(uint32(bc.MainLocals))
	w.u32(uint32(bc.MainMaxStack))
	if w.err != nil {
		return nil, w.err
	}
	return w.buf.Bytes(), nil
}

// Decode 解析 .xlc 内容。返回的 Bytecode 可以直接交给 VM 执行。
//
// 返回的 error 只有两种情况：ErrNotCache（当作没有缓存，回退源码编译）
// 或者更具体的解析错误（同样应当回退）。调用方不需要区分。
func Decode(data []byte) (*Bytecode, CacheMeta, error) {
	var meta CacheMeta
	r := &reader{buf: data}
	if !bytes.Equal(r.take(len(CacheMagic)), []byte(CacheMagic)) {
		return nil, meta, ErrNotCache
	}
	if v := r.u16(); v != CacheFormatVersion {
		return nil, meta, fmt.Errorf("%w: format version %d (want %d)", ErrNotCache, v, CacheFormatVersion)
	}
	flags := r.u8()
	meta.Optimized = flags&1 != 0
	if flags&^1 != 0 {
		return nil, meta, fmt.Errorf("%w: unknown flag bits %#x", ErrNotCache, flags)
	}
	copy(meta.SourceHash[:], r.bytes(32))
	meta.SourceSize = r.i64()
	meta.SourceMTime = r.i64()

	// 方法名表
	nMethods := r.count(maxMethods)
	syms := parser.NewSymTable("Base")
	for i := 0; i < nMethods; i++ {
		syms.Methods.Add(r.str(maxNameBytes))
	}

	nConst := r.count(maxConstants)
	constants := make([]object.Object, 0, capHint(nConst))
	for i := 0; i < nConst; i++ {
		obj, err := r.obj(0)
		if err != nil {
			return nil, meta, err
		}
		constants = append(constants, obj)
	}

	ins := r.ins()
	lines := r.lines()
	mainLocals := r.count(1 << 20)
	mainMaxStack := r.count(1 << 20)
	if r.err != nil {
		return nil, meta, r.err
	}

	bc := &Bytecode{
		Instruction:  ins,
		Lines:        lines,
		Constants:    constants,
		Symbols:      syms,
		MainLocals:   mainLocals,
		MainMaxStack: mainMaxStack,
	}
	if err := Validate(bc); err != nil {
		return nil, meta, fmt.Errorf("%w: %v", ErrNotCache, err)
	}
	return bc, meta, nil
}

// Validate 对一份（可能来自磁盘的）字节码做结构校验。
//
// 这是把 .xlc 当成不可信输入的最后一道闸门：指令流本身用 code.MaxStack 校验
// （未知操作码 / 越界跳转 / 栈下溢 / 汇合深度不一致都会被拒），
// 这里再补上常量下标、方法名下标与函数对象自身的检查。
func Validate(bc *Bytecode) error {
	if bc == nil {
		return errors.New("nil bytecode")
	}
	if len(bc.Instruction) == 0 {
		return errors.New("empty instruction stream")
	}
	if err := checkIns(bc.Instruction, bc.Constants, methodCount(bc.Symbols), bc.MainLocals, 0); err != nil {
		return err
	}
	if len(bc.Constants) > maxConstants {
		return fmt.Errorf("too many constants (%d)", len(bc.Constants))
	}
	for i, c := range bc.Constants {
		switch v := c.(type) {
		case object.CompiledFunc:
			if len(v.FreeVars) > 1<<16 {
				return fmt.Errorf("function %q: too many captured variables", v.FnName)
			}
			for _, free := range v.FreeVars {
				if free.Index < 0 || free.Index >= 1<<16 {
					return fmt.Errorf("function %q: bad capture index %d", v.FnName, free.Index)
				}
			}
			if err := checkIns(v.Instructions, bc.Constants, methodCount(bc.Symbols), v.LocalsNum, len(v.FreeVars)); err != nil {
				return fmt.Errorf("function %q: %v", v.FnName, err)
			}
			if v.LocalsNum < 0 || v.ParametersNum < 0 || v.ParametersNum > v.LocalsNum {
				return fmt.Errorf("function %q: bad locals/params (%d/%d)", v.FnName, v.LocalsNum, v.ParametersNum)
			}
		case object.Int, object.Float, object.String, object.Boolean, object.Null, *object.Array:
			// 正常常量
		case nil:
			return fmt.Errorf("constant %d is nil", i)
		default:
			return fmt.Errorf("constant %d has unsupported type %T", i, c)
		}
	}
	return nil
}

// checkIns 逐条解码指令，校验下标与跳转目标，并用数据流算出栈上界。
func checkIns(ins code.Instructions, constants []object.Object, nMethods, nLocals, nFree int) error {
	for i := 0; i < len(ins); {
		op := code.Opcode(ins[i])
		def, ok := code.Definitions[op]
		if !ok {
			return fmt.Errorf("unknown opcode %d at %d", op, i)
		}
		width := 0
		for _, w := range def.OperandWidth {
			width += w
		}
		if i+1+width > len(ins) {
			return fmt.Errorf("truncated operand for %s at %d", def.Name, i)
		}
		operand, _ := code.ReadOperand(def, ins[i+1:])
		switch op {
		case code.OpConstant:
			if operand[0] >= len(constants) {
				return fmt.Errorf("constant index %d out of range at %d", operand[0], i)
			}
		case code.OpClosure, code.OpCaptureClosure, code.OpCallFuncConst:
			if operand[0] >= len(constants) {
				return fmt.Errorf("callee index %d out of range at %d", operand[0], i)
			}
			fn, ok := constants[operand[0]].(object.CompiledFunc)
			if !ok {
				return fmt.Errorf("callee at %d is not a function", i)
			}
			if op != code.OpCaptureClosure && len(fn.FreeVars) != 0 {
				return fmt.Errorf("function %q requires a closure environment at %d", fn.FnName, i)
			}
			if op == code.OpCaptureClosure {
				for _, free := range fn.FreeVars {
					limit := nFree
					if free.Local {
						limit = nLocals
					}
					if free.Index < 0 || free.Index >= limit {
						return fmt.Errorf("capture index %d out of range at %d", free.Index, i)
					}
				}
			}
		case code.OpGetFree, code.OpSetFree:
			if operand[0] >= nFree {
				return fmt.Errorf("free variable index %d out of range at %d", operand[0], i)
			}
		case code.OpCallMethod:
			if operand[0] < 0 || operand[0] >= nMethods {
				return fmt.Errorf("method index %d out of range at %d", operand[0], i)
			}
		}
		i += 1 + width
	}
	// MaxStack 会做数据流分析：越界跳转、栈下溢、汇合处深度冲突都会报错。
	if _, err := code.MaxStack(ins); err != nil {
		return err
	}
	return nil
}

func methodCount(syms *parser.SymTable) int {
	if syms == nil || syms.Methods == nil {
		return 0
	}
	return len(syms.Methods.Names())
}

func methodNames(syms *parser.SymTable) []string {
	if syms == nil || syms.Methods == nil {
		return nil
	}
	return syms.Methods.Names()
}

// ---------------------------------------------------------------------------
// 编解码原语
// ---------------------------------------------------------------------------

type writer struct {
	buf *bytes.Buffer
	err error
}

func (w *writer) u8(v uint8) {
	if w.err != nil {
		return
	}
	w.buf.WriteByte(v)
}

func (w *writer) u16(v uint16) {
	if w.err != nil {
		return
	}
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	w.buf.Write(b[:])
}

func (w *writer) u32(v uint32) {
	if w.err != nil {
		return
	}
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	w.buf.Write(b[:])
}

func (w *writer) i64(v int64) {
	if w.err != nil {
		return
	}
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(v))
	w.buf.Write(b[:])
}

func (w *writer) bytes(p []byte) {
	if w.err != nil {
		return
	}
	w.buf.Write(p)
}

// blob 写原始字节（无长度前缀），用于固定布局的文件头。
func (w *writer) blob(p []byte) { w.bytes(p) }

// str 写一个 uint16 长度前缀的字符串。
func (w *writer) str(s string) {
	if w.err != nil {
		return
	}
	if len(s) > maxNameBytes {
		w.err = fmt.Errorf("xlc: string too long (%d bytes)", len(s))
		return
	}
	w.u16(uint16(len(s)))
	w.buf.WriteString(s)
}

// raw 写一个 uint32 长度前缀的字节串。
func (w *writer) raw(p []byte, limit int) {
	if w.err != nil {
		return
	}
	if len(p) > limit {
		w.err = fmt.Errorf("xlc: blob too long (%d bytes, limit %d)", len(p), limit)
		return
	}
	w.u32(uint32(len(p)))
	w.buf.Write(p)
}

func (w *writer) ins(ins code.Instructions) { w.raw(ins, maxInstructions) }

func (w *writer) lines(lines []code.LineEntry) {
	if w.err != nil {
		return
	}
	if len(lines) > maxLines {
		w.err = fmt.Errorf("xlc: too many line entries (%d)", len(lines))
		return
	}
	w.u32(uint32(len(lines)))
	for _, e := range lines {
		w.u32(uint32(e.Offset))
		w.u32(uint32(int32(e.Line)))
	}
}

func (w *writer) obj(o object.Object) error {
	if w.err != nil {
		return w.err
	}
	switch v := o.(type) {
	case object.Null:
		w.u8(tagNull)
	case object.Int:
		w.u8(tagInt)
		w.i64(int64(v.Value))
	case object.Float:
		w.u8(tagFloat)
		w.i64(int64(math.Float64bits(v.Value)))
	case object.Boolean:
		w.u8(tagBool)
		if v.Value {
			w.u8(1)
		} else {
			w.u8(0)
		}
	case object.String:
		w.u8(tagString)
		w.raw([]byte(string(v.Value)), maxStringBytes)
	case *object.Array:
		w.u8(tagArray)
		w.u32(uint32(len(v.Elements)))
		for _, e := range v.Elements {
			if err := w.obj(e); err != nil {
				return err
			}
		}
	case object.CompiledFunc:
		w.u8(tagFunc)
		w.str(v.FnName)
		w.u32(uint32(v.ParametersNum))
		w.u32(uint32(v.LocalsNum))
		w.u32(uint32(v.MaxStack))
		w.u32(uint32(int32(v.LineLoc)))
		w.u32(uint32(int32(v.ScopeID)))
		w.u32(uint32(len(v.FreeVars)))
		for _, free := range v.FreeVars {
			var local uint8
			if free.Local {
				local = 1
			}
			w.u8(local)
			w.u32(uint32(free.Index))
		}
		w.ins(v.Instructions)
		w.lines(v.Lines)
	default:
		return fmt.Errorf("xlc: cannot serialize constant of type %T", o)
	}
	return w.err
}

type reader struct {
	buf []byte
	pos int
	err error
}

func (r *reader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

func (r *reader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if n < 0 || r.pos+n > len(r.buf) {
		r.fail(fmt.Errorf("%w: truncated at offset %d (need %d bytes)", ErrNotCache, r.pos, n))
		return false
	}
	return true
}

func (r *reader) u8() uint8 {
	if !r.need(1) {
		return 0
	}
	v := r.buf[r.pos]
	r.pos++
	return v
}

func (r *reader) u16() uint16 {
	if !r.need(2) {
		return 0
	}
	v := binary.LittleEndian.Uint16(r.buf[r.pos:])
	r.pos += 2
	return v
}

func (r *reader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	v := binary.LittleEndian.Uint32(r.buf[r.pos:])
	r.pos += 4
	return v
}

func (r *reader) i64() int64 {
	if !r.need(8) {
		return 0
	}
	v := int64(binary.LittleEndian.Uint64(r.buf[r.pos:]))
	r.pos += 8
	return v
}

func (r *reader) bytes(n int) []byte {
	if !r.need(n) {
		return nil
	}
	v := r.buf[r.pos : r.pos+n]
	r.pos += n
	return v
}

// take 读固定长度的原始字节（无长度前缀）。
func (r *reader) take(n int) []byte { return r.bytes(n) }

// count 读一个 uint32 计数并做上限检查。
func (r *reader) count(limit int) int {
	v := r.u32()
	if r.err != nil {
		return 0
	}
	if v > uint32(limit) {
		r.fail(fmt.Errorf("%w: count %d exceeds limit %d", ErrNotCache, v, limit))
		return 0
	}
	return int(v)
}

func (r *reader) str(limit int) string {
	n := int(r.u16())
	if r.err != nil {
		return ""
	}
	if n > limit {
		r.fail(fmt.Errorf("%w: string length %d exceeds limit %d", ErrNotCache, n, limit))
		return ""
	}
	return string(r.bytes(n))
}

func (r *reader) raw(limit int) []byte {
	n := r.count(limit)
	if r.err != nil {
		return nil
	}
	return r.bytes(n)
}

func (r *reader) ins() code.Instructions { return code.Instructions(r.raw(maxInstructions)) }

func (r *reader) lines() []code.LineEntry {
	n := r.count(maxLines)
	if r.err != nil {
		return nil
	}
	out := make([]code.LineEntry, 0, capHint(n))
	for i := 0; i < n; i++ {
		off := r.u32()
		line := int32(r.u32())
		if r.err != nil {
			return nil
		}
		out = append(out, code.LineEntry{Offset: int(off), Line: int(line)})
	}
	return out
}

func (r *reader) obj(depth int) (object.Object, error) {
	if r.err != nil {
		return nil, r.err
	}
	if depth > 8 {
		return nil, fmt.Errorf("%w: constant nesting too deep", ErrNotCache)
	}
	switch r.u8() {
	case tagNull:
		return object.Null{}, nil
	case tagInt:
		return object.Int{Value: int(r.i64())}, nil
	case tagFloat:
		return object.Float{Value: math.Float64frombits(uint64(r.i64()))}, nil
	case tagBool:
		return object.Boolean{Value: r.u8() != 0}, nil
	case tagString:
		raw := r.raw(maxStringBytes)
		if r.err != nil {
			return nil, r.err
		}
		return object.String{Value: []rune(string(raw))}, nil
	case tagArray:
		n := r.count(maxConstants)
		if r.err != nil {
			return nil, r.err
		}
		elems := make([]object.Object, 0, capHint(n))
		for i := 0; i < n; i++ {
			e, err := r.obj(depth + 1)
			if err != nil {
				return nil, err
			}
			elems = append(elems, e)
		}
		return &object.Array{Elements: elems}, nil
	case tagFunc:
		fn := object.CompiledFunc{}
		fn.FnName = r.str(maxNameBytes)
		fn.ParametersNum = r.count(1 << 16)
		fn.LocalsNum = r.count(1 << 16)
		fn.MaxStack = r.count(1 << 16)
		fn.LineLoc = int(int32(r.u32()))
		fn.ScopeID = int(int32(r.u32()))
		nFree := r.count(1 << 16)
		for i := 0; i < nFree; i++ {
			local := r.u8()
			if local > 1 {
				return nil, fmt.Errorf("%w: invalid capture kind %d", ErrNotCache, local)
			}
			index := r.count((1 << 16) - 1)
			fn.FreeVars = append(fn.FreeVars, object.FreeVar{Local: local == 1, Index: index})
		}
		fn.Instructions = r.ins()
		fn.Lines = r.lines()
		if r.err != nil {
			return nil, r.err
		}
		return fn, nil
	}
	return nil, fmt.Errorf("%w: unknown constant tag", ErrNotCache)
}

// capHint 把预分配容量限制在合理范围内：畸形文件声明的巨大计数不会立刻
// 触发一次对应的巨大分配（真正的上限由 reader.count 负责拒绝）。
func capHint(n int) int {
	const limit = 4096
	if n < 0 {
		return 0
	}
	if n > limit {
		return limit
	}
	return n
}
