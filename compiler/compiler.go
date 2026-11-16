package compiler

import (
	"Interpreter/ast"
	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/errors"
	"Interpreter/ir"
	"Interpreter/object"
	"Interpreter/parser"
	"Interpreter/tokens"
	"fmt"
	"strconv"
)

type Compiler struct {
	*errors.Errors

	debug       bool
	constants   *ConstTable
	scope       []CompilationScope
	scopeIdx    int
	symTable    *parser.SymTable
	interpreter bool
	tmpOpPos    []InsPosInfo
	// loops 是"当前嵌套的循环"栈。break 只回填到最内层循环的结束位置，
	// 每层循环出栈时各自回填自己的 break，互不干扰。
	loops []loopCtx
	// curFuncID 是当前正在编译的函数作用域编号（顶层为 0），
	// 用于定位闭包捕获的局部变量。
	curFuncID int
	// fnIdx 记录"函数作用域编号 -> 常量池里的函数下标"，
	// 让同名函数在不同作用域里能各自解析到自己的函数对象。
	fnIdx map[int]int
	// line 是当前正在编译的 AST 节点的源码行，emit 用它生成行号映射
	line int
	// optimize 控制是否在编译结束后跑 IR/SSA 优化管线
	optimize bool
	// result 缓存优化后的字节码；非 nil 时 ByteCode 直接返回它
	result *bytecode.Bytecode
	// optimizeErr 记录优化器放弃的原因（仅诊断用，不影响正确性）
	optimizeErr error
}

func NewScope() CompilationScope {
	s := CompilationScope{
		instructions: code.Instructions{},
		lastIns:      EmittedIns{},
		prevIns:      EmittedIns{},
	}
	return s
}

func NewCompiler() *Compiler {
	rootScope := NewScope()

	return &Compiler{
		Errors:    errors.NewErr(),
		debug:     false,
		constants: NewConstTable(),
		scope:     []CompilationScope{rootScope},
		scopeIdx:  0,
		tmpOpPos:  []InsPosInfo{},
		fnIdx:     map[int]int{},
		optimize:  true,
	}
}

// SetOptimize 打开/关闭 IR 优化管线。默认开启。
func (c *Compiler) SetOptimize(on bool) {
	c.optimize = on
}

func (c *Compiler) SetMode() {
	c.interpreter = !c.interpreter
}

func (c *Compiler) SetSymbol(table *parser.SymTable) {
	c.symTable = table
	for k, v := range object.BuiltinFns {
		c.symTable.DefineBuiltin(v.Name, k)
	}
}

func (c *Compiler) Debug() {
	c.debug = true
	if c.debug {
		if c.HasError() {
			fmt.Println("ERROR:", c.Errs())
			return
		}
		fmt.Print(c.ByteCode().Ins())
	}
}

// compileLogicExpr 用跳转实现 and/or 的短路求值：
// a and b 只在 a 为真时求值 b，a or b 只在 a 为假时求值 b。
//
// 结果先写进匿名临时槽位再读出来，而不是留在操作数栈上跨基本块。
// 这样"每个基本块入口/出口操作数栈为空"成为全局不变式，
// 后续 IR 构造（stack-to-register promotion）与 SSA 都简单得多。
func (c *Compiler) compileLogicExpr(node ast.InfixExpr, optimize bool) {
	tmp := c.symTable.AllocLocal()

	c.compile(node.Left, optimize)
	if node.Op.Type == tokens.And {
		jumpLeftFalse := c.emit(code.OpJumpNotTrue, 9999)
		c.compile(node.Right, optimize)
		jumpRightFalse := c.emit(code.OpJumpNotTrue, 9999)
		c.emit(code.OpTrue)
		c.emit(code.OpSetLocal, tmp)
		jumpEnd := c.emit(code.OpJump, 9999)

		falsePos := len(c.curInstruction())
		c.changeOperand(jumpLeftFalse, falsePos)
		c.changeOperand(jumpRightFalse, falsePos)
		c.emit(code.OpFalse)
		c.emit(code.OpSetLocal, tmp)

		c.changeOperand(jumpEnd, len(c.curInstruction()))
		c.scope[c.scopeIdx].straightLine = false
		c.emit(code.OpGetLocal, tmp)
		return
	}

	jumpLeftFalse := c.emit(code.OpJumpNotTrue, 9999)
	c.emit(code.OpTrue)
	c.emit(code.OpSetLocal, tmp)
	jumpEndTrue := c.emit(code.OpJump, 9999)

	c.changeOperand(jumpLeftFalse, len(c.curInstruction()))
	c.compile(node.Right, optimize)
	jumpRightFalse := c.emit(code.OpJumpNotTrue, 9999)
	c.emit(code.OpTrue)
	c.emit(code.OpSetLocal, tmp)
	jumpEndTrue2 := c.emit(code.OpJump, 9999)

	falsePos := len(c.curInstruction())
	c.changeOperand(jumpRightFalse, falsePos)
	c.emit(code.OpFalse)
	c.emit(code.OpSetLocal, tmp)

	endPos := len(c.curInstruction())
	c.changeOperand(jumpEndTrue, endPos)
	c.changeOperand(jumpEndTrue2, endPos)
	c.scope[c.scopeIdx].straightLine = false
	c.emit(code.OpGetLocal, tmp)
}

// compile 负责把 AST 翻译成字节码。
// 约定：表达式在栈上留下 1 个值，语句（if/for/break/def/赋值/var/return）不留下值。
func (c *Compiler) compile(node ast.Node, optimize bool) {
	if node == nil {
		return
	}
	if l := node.Line(); l > 0 {
		c.line = l
	}
	switch node := node.(type) {
	case ast.Program:
		for _, s := range node.Statements {
			c.compile(s, optimize)
		}
	case *ast.BlockStatement:
		c.scope[c.scopeIdx].straightLine = false
		saved := c.switchScope(node.ScopeID)
		for _, s := range node.Statements {
			c.compile(s, optimize)
		}
		c.symTable = saved
	case ast.ExprStatement:
		if node.Expression == nil {
			return
		}
		c.compile(node.Expression, optimize)
		// 语句级表达式的值必须立刻丢弃，否则循环里每轮泄漏一个栈槽，
		// 两千次迭代后就 stack overflow。被丢弃的值由 VM 记录，供 REPL 使用。
		if producesValue(node.Expression) {
			c.emit(code.OpPop)
		}
	case ast.FuncStatement:
		c.compile(node.Expression, optimize)
	case ast.IntNode:
		numObj := object.Int{Value: node.Value}
		consIdx := c.constants.AddObj(numObj)
		c.emit(code.OpConstant, consIdx)
	case ast.FloatNode:
		numObj := object.Float{Value: node.Value}
		consIdx := c.constants.AddObj(numObj)
		c.emit(code.OpConstant, consIdx)
	case ast.StringNode:
		strObj := object.String{Value: []rune(node.Value)}
		consIdx := c.constants.AddObj(strObj)
		c.emit(code.OpConstant, consIdx)
	case ast.BooleanNode:
		value := node.Value
		if value {
			c.emit(code.OpTrue)
		} else {
			c.emit(code.OpFalse)
		}
	case ast.NoneNode:
		c.emit(code.OpNull)
	case ast.PrefixExpr:
		c.compile(node.Right, optimize)
		switch node.Op.Type {
		case tokens.Plus:
			c.emit(code.OpPlus)
		case tokens.Minus:
			c.emit(code.OpMinus)
		case tokens.Not:
			c.emit(code.OpNot)
		default:
			c.NewErrorF("unknown operator %s", node.Op.Str())
		}
	case ast.InfixExpr:
		if node.Op.Type == tokens.And || node.Op.Type == tokens.Or {
			c.compileLogicExpr(node, optimize)
			return
		}
		switch node.Op.Type {
		case tokens.IPlus, tokens.IMinus, tokens.IMul, tokens.IDiv, tokens.IMod, tokens.IPow, tokens.IFloorDiv:
			ident := node.Left
			s, ok := c.symTable.Resolve(ident.TokenLiteral())
			if !ok {
				c.NewErrorF("Identifier %s was not defined", s.Name)
			}
			c.compile(node.Left, optimize)
			c.compile(node.Right, optimize)
			switch node.Op.Type {
			case tokens.IPlus:
				c.emit(code.OpAdd)
			case tokens.IMinus:
				c.emit(code.OpSub)
			case tokens.IMul:
				c.emit(code.OpMul)
			case tokens.IDiv:
				c.emit(code.OpDiv)
			case tokens.IPow:
				c.emit(code.OpPow)
			case tokens.IMod:
				c.emit(code.OpMod)
			case tokens.IFloorDiv:
				c.emit(code.OpFloorDiv)
			}
			c.setScope(s)
		default:
			c.compile(node.Left, optimize)
			c.compile(node.Right, optimize)
			switch node.Op.Type {
			case tokens.Plus:
				c.emit(code.OpAdd)
			case tokens.Minus:
				c.emit(code.OpSub)
			case tokens.Mul:
				c.emit(code.OpMul)
			case tokens.Div:
				c.emit(code.OpDiv)
			case tokens.Mod:
				c.emit(code.OpMod)
			case tokens.FloorDiv:
				c.emit(code.OpFloorDiv)
			case tokens.Pow:
				c.emit(code.OpPow)
			case tokens.GT:
				c.emit(code.OpGT)
			case tokens.GTEq:
				c.emit(code.OpGTEq)
			case tokens.LT:
				c.emit(code.OpLT)
			case tokens.LTEq:
				c.emit(code.OpLTEq)
			case tokens.Equal:
				c.emit(code.OpEqual)
			case tokens.NotEq:
				c.emit(code.OpNotEQ)
			default:
				c.NewErrorF("unknown operator %s", node.Op.Str())
			}
		}
	case ast.IfExpression:
		c.compile(node.Condition, optimize)
		jumpNotTruePos := c.emit(code.OpJumpNotTrue, 9999)
		c.compile(node.Consequence, optimize)
		jumpPos := c.emit(code.OpJump, 9999)
		afterConSeqPos := len(c.curInstruction())
		c.changeOperand(jumpNotTruePos, afterConSeqPos)
		if node.Alternative != nil {
			c.compile(node.Alternative, optimize)
		}
		afterAlterPos := len(c.curInstruction())
		c.changeOperand(jumpPos, afterAlterPos)
		// if 是汇合点：往后的取值不能再沿用分支里的栈上值
		c.scope[c.scopeIdx].straightLine = false
	case ast.ForExpression:
		saved := c.switchScope(node.ScopeID)
		if node.InitCond != nil {
			c.compile(node.InitCond, false)
		}
		c.scope[c.scopeIdx].straightLine = false // 循环头是回边目标
		forStartPos := len(c.curInstruction())
		if node.Condition != nil {
			c.compile(node.Condition, false)
		} else {
			// for(;;) 这类无显式条件的循环等价于"永远成立"，
			// 必须压入一个真值，否则条件跳转会在空栈上取值
			c.emit(code.OpTrue)
		}
		breakPos := c.emit(code.OpJumpNotTrue, 9999)
		c.loops = append(c.loops, loopCtx{})
		c.compile(node.Loop, true)
		if node.EachOperate != nil {
			c.compile(node.EachOperate, false)
		}
		c.emit(code.OpJump, forStartPos)
		forEndPos := len(c.curInstruction())
		c.changeOperand(breakPos, forEndPos)
		// 只回填本层循环收集到的 break，外层循环的 break 由外层自己处理
		ctx := c.loops[len(c.loops)-1]
		c.loops = c.loops[:len(c.loops)-1]
		for _, pos := range ctx.breaks {
			c.changeOperand(pos, forEndPos)
		}
		c.emit(code.OpNull)
		c.emit(code.OpPop)
		c.scope[c.scopeIdx].straightLine = false
		c.symTable = saved
	case ast.BreakExpr:
		if len(c.loops) == 0 {
			c.NewErrorF("break outside of a loop")
			return
		}
		pos := c.emit(code.OpJump, 9999)
		top := len(c.loops) - 1
		c.loops[top].breaks = append(c.loops[top].breaks, pos)
	case ast.MethodCall:
		c.compile(node.Left, optimize)
		for i, method := range node.Methods {
			if i >= len(node.Arguments) {
				c.NewErrorF("internal: method call arguments out of sync")
				return
			}
			ident, ok := method.(ast.MethodNode)
			if !ok {
				c.NewErrorF("internal: method call without a method name")
				return
			}
			idx, ok := c.symTable.Methods.FindIdx(ident.Value)
			if !ok {
				c.NewErrorF("can't find method %s.", strconv.Quote(ident.Value))
				return
			}
			for _, args := range node.Arguments[i] {
				c.compile(args, optimize)
			}
			// 方法名下标 + 实参个数：实现在运行时按接收者类型查表得到
			c.emit(code.OpCallMethod, idx, len(node.Arguments[i]))
			if ident, ok := node.Left.(ast.IdentNode); ok {
				name := ident.Value
				s, ok := c.symTable.Resolve(name)
				if !ok {
					c.NewErrorF("node name %s can't found.", name)
					return
				}
				c.setScope(s)
			} else {
				c.emit(code.OpPop)
			}
		}
	case ast.MethodNode:
		// 方法名在 ast.MethodCall 里直接解析成 OpCallMethod 的操作数，
		// 不会单独发射指令；走到这里说明语法树形状不对。
		c.NewErrorF("internal: method name %s used outside a method call", strconv.Quote(node.Value))
	case ast.VarStatement:
		c.compile(node.Value, optimize)
		s, ok := c.symTable.Resolve(node.Indent.Value)
		if !ok {
			c.NewErrorF("undefined variable %s.", strconv.Quote(node.Indent.Value))
		}
		c.setScope(s)
	case ast.VarMethodCall:
		startPos := len(c.curInstruction())
		c.compile(node.Value, optimize)
		if c.isLastIns(code.OpPop) && c.curScope().lastIns.offset >= startPos {
			c.removeLastOp()
		}
		s, ok := c.symTable.Resolve(node.Indent.Value)
		if !ok {
			c.NewErrorF("undefined variable %s.", strconv.Quote(node.Indent.Value))
		}
		c.setScope(s)
	case ast.IdentNode:
		s, ok := c.symTable.Resolve(node.Value)
		if !ok {
			c.NewErrorF("undefined Identifier %s.", strconv.Quote(node.Str()))
			return
		}
		if s.Type == parser.F && s.ScopeType == parser.Global {
			idx, ok := c.fnIdx[s.ScopeID]
			if !ok {
				c.NewErrorF("function %s is used before it is defined.", strconv.Quote(node.Value))
				return
			}
			c.emit(code.OpClosure, idx)
		} else {
			c.getScope(s, optimize)
		}
	case ast.AssignStatement:
		c.compile(node.Statement, optimize)
		s, ok := c.symTable.Resolve(node.Identifier.Value)
		if !ok {
			c.NewErrorF("variable %s is undefined but used.", strconv.Quote(node.Identifier.Value))
		} else {
			c.setScope(s)
		}
	case ast.FuncDef:
		c.enterScope()
		savedTable := c.symTable
		savedFunc, savedLoops := c.curFuncID, c.loops
		// 用作用域编号定位这张函数表：同名函数各自有独立的表和槽位
		c.symTable = savedTable.FindScope(node.ScopeID)
		if c.symTable == nil {
			c.NewErrorF("internal: function scope %d not found", node.ScopeID)
			c.symTable = savedTable
			return
		}
		c.curFuncID = node.ScopeID
		c.scope[c.scopeIdx].funcID = node.ScopeID
		// 函数体是独立的控制流，不能继承外层的循环（函数里 break 是错误）
		c.loops = nil
		paramsCount := len(node.Parameters)
		fnIdx := c.constants.RegFunc(node.Name, paramsCount)
		c.fnIdx[node.ScopeID] = fnIdx
		c.compile(node.FuncBody, optimize)
		if c.isLastIns(code.OpPop) {
			c.replaceLast(code.OpReturnVal)
		}
		if !c.isLastIns(code.OpReturnVal) {
			c.emit(code.OpNull)
			c.emit(code.OpReturnVal)
		}
		numLocals := c.symTable.LocalSlots()
		lines := c.curScope().lines
		freeVars := c.curScope().freeVars
		instructions := c.leaveScope()

		compiledFn := object.CompiledFunc{
			FnName:        node.Name,
			Instructions:  instructions,
			Lines:         lines,
			LocalsNum:     numLocals,
			ParametersNum: paramsCount,
			LineLoc:       node.Token.Loc.Line,
			ScopeID:       node.ScopeID,
			FreeVars:      freeVars,
		}
		err := c.constants.AddFunc(fnIdx, compiledFn)
		if err != nil {
			c.Push(err)
		}
		c.symTable = savedTable
		c.curFuncID, c.loops = savedFunc, savedLoops
		if s, ok := c.symTable.Resolve(node.Name); ok && s.ScopeType == parser.Local {
			if len(freeVars) > 0 || fnIdx > operandLimit(code.OpClosure, 0) {
				c.emit(code.OpCaptureClosure, fnIdx)
			} else {
				c.emit(code.OpClosure, fnIdx)
			}
			c.setScope(s)
		}
	case ast.ReturnStatement:
		if node.ReturnVal != nil {
			c.compile(node.ReturnVal, optimize)
		} else {
			c.emit(code.OpNull)
		}
		c.emit(code.OpReturnVal)
	case ast.Array:
		for _, e := range node.Elements {
			c.compile(e, optimize)
		}
		c.emit(code.OpBuildArray, len(node.Elements))
	case ast.IndexSlice:
		if node.Start != nil {
			c.compile(node.Start, optimize)
		} else {
			c.emit(code.OpNull)
		}
		if node.End != nil {
			c.compile(node.End, optimize)
		} else {
			c.emit(code.OpNull)
		}
		if node.Step != nil {
			c.compile(node.Step, optimize)
		} else {
			c.emit(code.OpNull)
		}
		c.emit(code.OpMakeSlice)
	case ast.IndexExpression:
		c.compile(node.Left, optimize)
		c.compile(node.Index, optimize)
		c.emit(code.OpIndex)
	case ast.FuncCallExpr:
		c.compile(node.Function, optimize)
		for _, arg := range node.Arguments {
			c.compile(arg, optimize)
		}
		c.emit(code.OpCallFunc, len(node.Arguments))
	case ast.Map:
		for i := 0; i < len(node.Keys); i++ {
			c.compile(node.Keys[i], optimize)
			c.compile(node.Items[i], optimize)
		}
		c.emit(code.OpMakeMap, len(node.Keys)*2)
	case ast.ExpressionAssign:
		c.compile(node.New, optimize)
		c.compile(node.Old, optimize)
		c.compile(node.Key, optimize)
		c.emit(code.OpUpdate)
		s, _ := c.symTable.Resolve(node.Old.TokenLiteral())
		c.setScope(s)
	default:
		c.NewErrorF("unknown ast type %T.", node)
	}
}

func (c *Compiler) Compile(node ast.Node) {
	c.result = nil
	c.compile(node, true)
	c.handleNoCall()
	c.optimizeByteCode()
}

// optimizeByteCode 在编译成功后跑一遍 IR/SSA 优化。
//
// 优化器是"尽力而为"的契约：每个函数要么被等价改写，要么保持原样，
// 因此返回的字节码**总是**行为等价的，可以无条件采用；error 只带出
// 第一个放弃原因，用于诊断。
func (c *Compiler) optimizeByteCode() {
	if !c.optimize || c.HasError() || c.scopeIdx != 0 {
		return
	}
	raw := c.buildByteCode()
	opt, err := ir.Optimize(raw)
	c.result = opt
	c.optimizeErr = err
}

// OptimizeErr 返回优化期间放弃的原因（没有放弃时为 nil）。
// 它不影响正确性，只供测试与排障使用。
func (c *Compiler) OptimizeErr() error {
	return c.optimizeErr
}

func (c *Compiler) handleNoCall() {
	if c.interpreter {
		if c.isLastIns(code.OpPop) {
			c.replaceLast(code.OpPrintTop)
		} else {
			c.emit(code.OpPrintTop)
		}
	}
}

// producesValue 判断某个表达式编译后是否会在操作数栈上留下一个值。
// if/for/break/def 在本语言里是语句语义，栈高度不变；
// 复合赋值（a += b）也是语句：编译器会先算出结果再写回变量，
// 不会在栈上留下值，所以不能再补 OpPop。
func producesValue(node ast.Node) bool {
	switch n := node.(type) {
	case ast.IfExpression, ast.ForExpression, ast.BreakExpr, ast.FuncDef:
		return false
	case ast.InfixExpr:
		switch n.Op.Type {
		case tokens.IPlus, tokens.IMinus, tokens.IMul,
			tokens.IDiv, tokens.IFloorDiv, tokens.IPow, tokens.IMod:
			return false
		}
	}
	return true
}

// operandLimit 返回某个操作码第 idx 个操作数能表示的最大值。
// 操作数按大端定宽写入，超宽会被截断，所以必须在编译期检查。
func operandLimit(op code.Opcode, idx int) int {
	def, ok := code.Definitions[op]
	if !ok || idx >= len(def.OperandWidth) {
		return 0
	}
	return 1<<(8*def.OperandWidth[idx]) - 1
}

// checkOperand 校验操作数能否放进声明的位宽里，越界就报编译错误。
func (c *Compiler) checkOperand(op code.Opcode, operand []int) bool {
	for i, o := range operand {
		limit := operandLimit(op, i)
		if o < 0 || o > limit {
			c.NewErrorF("%s operand %d exceeds its %d-byte encodable range [0,%d]",
				code.Definitions[op].Name, o, code.Definitions[op].OperandWidth[i], limit)
			return false
		}
	}
	return true
}

func (c *Compiler) emit(op code.Opcode, operand ...int) int {
	pos := len(c.curInstruction())
	if !c.checkOperand(op, operand) {
		return pos
	}
	ins := code.Make(op, operand...)
	c.scope[c.scopeIdx].instructions = append(c.curInstruction(), ins...)
	if c.line > 0 {
		lines := c.scope[c.scopeIdx].lines
		if len(lines) == 0 || lines[len(lines)-1].Line != c.line {
			c.scope[c.scopeIdx].lines = append(lines, code.LineEntry{Offset: pos, Line: c.line})
		}
	}
	c.setLastIns(op, pos)
	switch op {
	case code.OpJump, code.OpJumpNotTrue, code.OpReturnVal:
		c.scope[c.scopeIdx].straightLine = false
	default:
		c.scope[c.scopeIdx].straightLine = true
	}
	return pos
}

func (c *Compiler) setLastIns(op code.Opcode, pos int) {
	prevIns := c.scope[c.scopeIdx].lastIns
	last := EmittedIns{op: op, offset: pos}
	c.scope[c.scopeIdx].prevIns = prevIns
	c.scope[c.scopeIdx].lastIns = last
}

func (c *Compiler) ByteCode() *bytecode.Bytecode {
	if c.HasError() {
		return &bytecode.Bytecode{}
	}
	if c.result != nil {
		return c.result
	}
	return c.buildByteCode()
}

func (c *Compiler) buildByteCode() *bytecode.Bytecode {
	ct := c.symTable
	for ct.Outer != nil {
		ct = ct.Outer
	}
	byCode := &bytecode.Bytecode{
		Instruction: c.curInstruction(),
		Lines:       c.curScope().lines,
		Constants:   c.constants.Store,
		Symbols:     ct,
		MainLocals:  ct.LocalSlots(),
	}
	// 编译期算出操作数栈上界（CPython 的 co_stacksize），
	// VM 建帧时按它预留，执行循环里就不用逐条指令检查容量。
	byCode.MainMaxStack = code.ConservativeMaxStack(byCode.Instruction)
	for i, obj := range byCode.Constants {
		fn, ok := obj.(object.CompiledFunc)
		if !ok {
			continue
		}
		fn.MaxStack = code.ConservativeMaxStack(fn.Instructions)
		byCode.Constants[i] = fn
	}
	return byCode
}

func (c *Compiler) isLastIns(op code.Opcode) bool {
	if len(c.curInstruction()) == 0 {
		return false
	}
	return c.curScope().lastIns.op == op
}

func (c *Compiler) removeLastOp() {
	c.scope[c.scopeIdx].instructions = c.curInstruction()[:c.curScope().lastIns.offset]
	c.scope[c.scopeIdx].lastIns = c.curScope().prevIns
}

func (c *Compiler) replaceIns(offset int, newIns []byte) {
	for i := 0; i < len(newIns); i++ {
		c.scope[c.scopeIdx].instructions[offset+i] = newIns[i]
	}
}

func (c *Compiler) changeOperand(opPos int, operand int) {
	op := code.Opcode(c.curInstruction()[opPos])
	if !c.checkOperand(op, []int{operand}) {
		return
	}
	newIns := code.Make(op, operand)
	c.replaceIns(opPos, newIns)
}

func (c *Compiler) curScope() CompilationScope {
	return c.scope[c.scopeIdx]
}

func (c *Compiler) enterScope() {
	s := NewScope()
	c.scope = append(c.scope, s)
	c.scopeIdx++
}

func (c *Compiler) leaveScope() code.Instructions {
	instructions := c.curInstruction()
	c.scope = c.scope[:len(c.scope)-1]
	c.scopeIdx--
	return instructions
}

func (c *Compiler) curInstruction() code.Instructions {
	return c.curScope().instructions
}

// switchScope 把符号表切到解析期为该块建立的那张表上，
// 返回切换前的表，供调用方恢复。
func (c *Compiler) switchScope(id int) *parser.SymTable {
	saved := c.symTable
	if id > 0 {
		if st := c.symTable.FindScope(id); st != nil {
			c.symTable = st
		}
	}
	return saved
}

// replaceLast 只替换最后一条指令的操作码，保留其操作数。
// 调用方必须保证新旧操作码的操作数宽度一致（Pop→ReturnVal/PrintTop、
// Set→Update 都满足）。用 code.Make 重建会把操作数清零。
func (c *Compiler) replaceLast(target code.Opcode) {
	lastPos := c.scope[c.scopeIdx].lastIns.offset
	c.scope[c.scopeIdx].instructions[lastPos] = byte(target)
	c.scope[c.scopeIdx].lastIns.op = target
}

func (c *Compiler) getScope(s parser.Symbol, optimize bool) {
	if s.ScopeType == parser.Local && s.FuncID != c.curFuncID {
		c.emit(code.OpGetFree, c.capture(c.scopeIdx, s))
		return
	}
	switch s.ScopeType {
	case parser.Global:
		if optimize && c.curScope().straightLine && c.isLastIns(code.OpSetGlobal) &&
			code.ReadUint16(c.scope[c.scopeIdx].instructions[len(c.curInstruction())-2:]) == uint16(s.Id) {
			// "刚存完又立刻读"融合成更新：值留在栈上。
			// 必须同步 lastIns，否则下一次取值会误判成还能再省一次。
			c.replaceLast(code.OpUpdateGlobal)
		} else {
			c.emit(code.OpGetGlobal, s.Id)
		}
	case parser.Local:
		if optimize && c.curScope().straightLine && c.isLastIns(code.OpSetLocal) &&
			code.ReadUint16(c.scope[c.scopeIdx].instructions[len(c.curInstruction())-2:]) == uint16(s.Id) {
			c.replaceLast(code.OpUpdateLocal)
		} else {
			c.emit(code.OpGetLocal, s.Id)
		}
	case parser.BuiltIn:
		c.emit(code.OpGetBuiltin, s.Id)
	}
}

func (c *Compiler) setScope(s parser.Symbol) {
	if s.ScopeType == parser.Local && s.FuncID != c.curFuncID {
		c.emit(code.OpSetFree, c.capture(c.scopeIdx, s))
		return
	}
	switch s.ScopeType {
	case parser.Global:
		c.emit(code.OpSetGlobal, s.Id)
	case parser.Local:
		c.emit(code.OpSetLocal, s.Id)
	}
}

// Intermediate functions forward cells even when only a deeper function uses them.
func (c *Compiler) capture(scope int, s parser.Symbol) int {
	for i, captured := range c.scope[scope].captures {
		if captured == s {
			return i
		}
	}
	if scope == 0 {
		c.NewErrorF("internal: cannot capture variable %s from function %d", s.Name, s.FuncID)
		return 0
	}
	free := object.FreeVar{Local: c.scope[scope-1].funcID == s.FuncID, Index: s.Id}
	if !free.Local {
		free.Index = c.capture(scope-1, s)
	}
	idx := len(c.scope[scope].captures)
	c.scope[scope].captures = append(c.scope[scope].captures, s)
	c.scope[scope].freeVars = append(c.scope[scope].freeVars, free)
	return idx
}

type EmittedIns struct {
	op     code.Opcode
	offset int
}

type CompilationScope struct {
	instructions code.Instructions
	funcID       int
	captures     []parser.Symbol
	freeVars     []object.FreeVar
	// lines 是"指令偏移 -> 源码行"映射，只在行号变化时追加一条
	lines []code.LineEntry
	lastIns,
	prevIns EmittedIns
	// straightLine 表示"最后发射的那条指令在到达这里时一定刚刚执行过"。
	// 只有这种情况才允许把 OpSet 融合成 OpUpdate（值确实还在栈上）。
	// 跨过跳转/分支后它会被置为 false。
	straightLine bool
}
