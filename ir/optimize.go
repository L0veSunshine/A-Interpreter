package ir

import (
	"fmt"

	"Interpreter/bytecode"
	"Interpreter/cfg"
	"Interpreter/code"
	"Interpreter/object"
)

// Optimize 是整个优化器的唯一入口。
//
// 语义保证：返回的字节码要么与输入**行为等价**，要么那一部分保持原样。
// 优化是函数内局部变换，因此"尽力而为"：某个函数放弃只影响它自己，
// 其余函数照常优化；第一个失败原因通过 error 带出（仅作诊断用，调用方
// 可以直接忽略）。
//
// 目前会放弃优化的情形：
//   - REPL 的隐式打印指令（IR 还没建模）
//   - 块边界上操作数栈非空（需要栈上的 phi）
//   - 闭包捕获与共享变量访问（必须保留原始局部槽位）
//   - 任何一步分析自检失败（例如读未定义槽位）
func Optimize(bc *bytecode.Bytecode) (*bytecode.Bytecode, error) {
	pool := newConstPool(bc.Constants)

	// 先记下原始的函数对象数量：优化过程中常量池可能新增常量，
	// 但不会再新增函数，因此只处理原有的那些。
	funcCount := len(pool.objs)

	// main 优化失败不代表其它函数没救：main 中某个不支持的指令形态，
	// 不应阻止函数体内的热循环照常优化。所以这里改成"尽力而为"：
	// 失败的个体保留原样，成功的部分生效，同时把第一个失败原因带出去。
	var firstErr error

	mainIns, mainLines, mainLocals, err := optimizeFunction(
		"<main>", 0, bc.Instruction, bc.Lines, bc.MainLocals, pool, true)
	mainMaxStack := code.ConservativeMaxStack(mainIns)
	if err != nil {
		firstErr = err
		mainIns, mainLines, mainLocals = bc.Instruction, bc.Lines, bc.MainLocals
		mainMaxStack = bc.MainMaxStack
	}

	for i := 0; i < funcCount; i++ {
		fn, ok := pool.objs[i].(object.CompiledFunc)
		if !ok {
			continue
		}
		ins, lines, locals, ferr := optimizeFunction(
			fn.FnName, fn.ParametersNum, fn.Instructions, fn.Lines, fn.LocalsNum, pool, false)
		if ferr != nil {
			// 单个函数放弃优化：保持原样即可，不影响调用它的代码
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", fn.FnName, ferr)
			}
			continue
		}
		fn.Instructions = ins
		fn.Lines = lines
		fn.LocalsNum = locals
		// 指令流变了，操作数栈上界必须重算（融合会改变栈操作序列）
		fn.MaxStack = code.ConservativeMaxStack(ins)
		pool.objs[i] = fn
	}

	return &bytecode.Bytecode{
		Instruction:  mainIns,
		Lines:        mainLines,
		Constants:    pool.objs,
		Symbols:      bc.Symbols,
		MainLocals:   mainLocals,
		MainMaxStack: mainMaxStack,
	}, firstErr
}

// maxLocals 与 VM 里 2 字节槽位操作数的上限一致。
const maxLocals = 1 << 16

// FusionEnabled 控制是否启用超指令融合（默认开启）。
//
// 关掉它可以让同一份字节码走"不融合"的路径，用于对比收益或定位问题；
// 融合只是减少解释器派发次数，语义与行号都不变。
var FusionEnabled = true

// optimizeFunction 优化单个函数。prevLocals 是未优化时的槽位数，
// 放弃优化时必须原样返回，否则 VM 会给帧分配过少的槽位。
func optimizeFunction(name string, params int, ins code.Instructions, lines []code.LineEntry,
	prevLocals int, pool *constPool, isMain bool) (code.Instructions, []code.LineEntry, int, error) {
	if len(ins) == 0 {
		return ins, lines, prevLocals, nil
	}

	fn, err := buildFunction(name, params, ins, lines, pool, isMain)
	if err != nil {
		return nil, nil, 0, err
	}
	if fn == nil {
		return ins, lines, prevLocals, nil
	}
	if err := fn.runSSA(); err != nil {
		return nil, nil, 0, err
	}
	if err := fn.optimize(); err != nil {
		return nil, nil, 0, err
	}
	outIns, outLines, locals, err := fn.lower(isMain)
	if err != nil {
		return nil, nil, 0, err
	}
	// 最后一轮清理：去掉"跳到紧邻下一块"的冗余跳转、删除不可达块。
	// 注意必须用 CompactLines：压缩会改变偏移，行号表要一起重映射，
	// 否则运行时报错会指到错误的源码行。
	if g, gerr := cfg.Build(outIns); gerr == nil {
		if compacted, compactedLines, cerr := g.CompactLines(outIns, outLines); cerr == nil {
			outIns, outLines = compacted, compactedLines
		}
	}
	// 超指令融合：把常见的相邻指令合并成一条，减少解释器的派发次数。
	// 融合只减少派发，语义与行号都保持不变；失败时保留未融合的指令流。
	if FusionEnabled {
		if fused, fusedLines, ferr := code.Fuse(outIns, outLines); ferr == nil {
			outIns, outLines = fused, fusedLines
		}
	}
	if locals > maxLocals {
		return nil, nil, 0, bail("function %s needs %d local slots (max %d)", name, locals, maxLocals)
	}
	return outIns, outLines, locals, nil
}

// attachLines 已不再需要：行号在 build 阶段就按指令偏移精确绑定。
