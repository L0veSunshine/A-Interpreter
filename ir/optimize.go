package ir

import (
	"Interpreter/bytecode"
	"Interpreter/cfg"
	"Interpreter/code"
	"Interpreter/object"
)

// Optimize 是整个优化器的唯一入口。
//
// 语义保证：要么返回一份**行为等价**的新字节码，要么原样返回输入
// （同时把原因作为 error 带出）。调用方可以在出错时安全地忽略优化结果。
//
// 优化是函数内局部变换，因此单个函数放弃不会影响其它函数：
// 只要 main 能优化，就返回"尽可能优化过"的字节码；main 自己失败时
// 原样返回（此时调用方应当使用未优化的字节码）。
//
// 目前会放弃优化的情形：
//   - 方法调用 / 切片字面量 / REPL 的隐式打印指令（IR 还没建模）
//   - 块边界上操作数栈非空（需要栈上的 phi）
//   - 任何一步分析自检失败（例如读未定义槽位）
func Optimize(bc *bytecode.Bytecode) (*bytecode.Bytecode, error) {
	pool := newConstPool(bc.Constants)

	// 先记下原始的函数对象数量：优化过程中常量池可能新增常量，
	// 但不会再新增函数，因此只处理原有的那些。
	funcCount := len(pool.objs)

	mainIns, mainLines, mainLocals, err := optimizeFunction(
		"<main>", 0, bc.Instruction, bc.Lines, bc.MainLocals, pool, true)
	if err != nil {
		return bc, err
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
			continue
		}
		fn.Instructions = ins
		fn.Lines = lines
		fn.LocalsNum = locals
		pool.objs[i] = fn
	}

	return &bytecode.Bytecode{
		Instruction: mainIns,
		Lines:       mainLines,
		Constants:   pool.objs,
		Symbols:     bc.Symbols,
		MainLocals:  mainLocals,
	}, nil
}

// maxLocals 与 VM 里 2 字节槽位操作数的上限一致。
const maxLocals = 1 << 16

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
	// 最后一轮清理：去掉"跳到紧邻下一块"的冗余跳转、删除不可达块
	if g, gerr := cfg.Build(outIns); gerr == nil {
		if compacted, cerr := g.Compact(outIns); cerr == nil {
			outIns = compacted
		}
	}
	if locals > maxLocals {
		return nil, nil, 0, bail("function %s needs %d local slots (max %d)", name, locals, maxLocals)
	}
	return outIns, outLines, locals, nil
}

// attachLines 已不再需要：行号在 build 阶段就按指令偏移精确绑定。
