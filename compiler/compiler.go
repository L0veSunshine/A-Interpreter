package compiler

import (
	"Interpreter/ast"
	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/errors"
	"Interpreter/object"
	"Interpreter/parser"
	"Interpreter/tokens"
	"fmt"
	"reflect"
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
	}
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

func (c *Compiler) compile(node ast.Node, optimize bool) {
	switch node := node.(type) {
	case ast.Program:
		for _, s := range node.Statements {
			c.compile(s, optimize)
		}
	case *ast.BlockStatement:
		for _, s := range node.Statements {
			c.compile(s, optimize)
		}
	case ast.ExprStatement:
		c.compile(node.Expression, optimize)
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
		switch node.Op.Type {
		case tokens.LT, tokens.LTEq:
			c.compile(node.Right, optimize)
			c.compile(node.Left, optimize)
			switch node.Op.Type {
			case tokens.LT:
				c.emit(code.OpGT)
			case tokens.LTEq:
				c.emit(code.OpGTEq)
			default:
				c.NewErrorF("unsupported op %s", node.Op.Str())
			}
			return
		case tokens.IPlus, tokens.IMinus, tokens.IMul, tokens.IDiv, tokens.IMod, tokens.IPow:
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
			case tokens.Pow:
				c.emit(code.OpPow)
			case tokens.GT:
				c.emit(code.OpGT)
			case tokens.GTEq:
				c.emit(code.OpGTEq)
			case tokens.Equal:
				c.emit(code.OpEqual)
			case tokens.NotEq:
				c.emit(code.OpNotEQ)
			case tokens.And:
				c.emit(code.OpAnd)
			case tokens.Or:
				c.emit(code.OpOr)
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
			c.emit(code.OpPop)
		}
		afterAlterPos := len(c.curInstruction())
		c.changeOperand(jumpPos, afterAlterPos)
	case ast.ForExpression:
		if node.InitCond != nil {
			c.compile(node.InitCond, false)
		}
		forStartPos := len(c.curInstruction())
		c.compile(node.Condition, false)
		breakPos := c.emit(code.OpJumpNotTrue, 9999)
		c.compile(node.Loop, true)
		if node.EachOperate != nil {
			c.compile(node.EachOperate, false)
		}
		c.emit(code.OpJump, forStartPos)
		forEndPos := len(c.curInstruction())
		c.changeOperand(breakPos, forEndPos)
		for _, point := range c.tmpOpPos {
			if point.PType == BreakPoint {
				c.changeOperand(point.Pos, forEndPos)
			}
			if point.PType == LoopStart {
				c.changeOperand(point.Pos, forStartPos)
			}
		}
		c.tmpOpPos = append(c.tmpOpPos[:0])
		c.emit(code.OpNull)
		c.emit(code.OpPop)
	case ast.BreakExpr:
		pos := c.emit(code.OpJump, 9999)
		posInfo := InsPosInfo{
			Pos:   pos,
			PType: BreakPoint,
		}
		c.tmpOpPos = append(c.tmpOpPos, posInfo)
	case ast.MethodCall:
		c.compile(node.Left, optimize)
		for i, method := range node.Methods {
			c.compile(method, optimize)
			for _, args := range node.Arguments[i] {
				c.compile(args, optimize)
			}
			c.emit(code.OpCallMethod, len(node.Arguments[i]))
			if reflect.TypeOf(node.Left).Name() == "IdentNode" {
				name := node.Left.(ast.IdentNode).Value
				s, ok := c.symTable.Resolve(name)
				if !ok {
					c.NewErrorF("node name %s can't found.", name)
				}
				c.setScope(s)
			} else {
				c.emit(code.OpPop)
			}
		}
	case ast.MethodCallStmt:
		c.compile(node.Call, optimize)
		c.emit(code.OpPop)
	case ast.MethodNode:
		s, ok := c.symTable.Methods.FindIdx(node.Value)
		if !ok {
			c.NewErrorF("can't find method %s.", strconv.Quote(node.Str()))
		}
		c.emit(code.OpLoadMethod, s)
	case ast.VarStatement:
		c.compile(node.Value, optimize)
		s, ok := c.symTable.Resolve(node.Indent.Value)
		if !ok {
			c.NewErrorF("undefined variable %s.", strconv.Quote(node.Indent.Value))
		}
		c.setScope(s)
	case ast.VarMethodCall:
		c.compile(node.Value, optimize)
		if c.isLastIns(code.OpPop) {
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
		}
		if s.Type == parser.F && s.ScopeType != parser.BuiltIn {
			idx, ok := c.constants.Find(s.Name)
			if !ok {
				c.NewErrorF("undefined func %s.", strconv.Quote(node.Value))
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
		c.symTable = parser.Search(node.Name, c.symTable)
		paramsCount := len(node.Parameters)
		fnIdx := c.constants.RegFunc(node.Name, paramsCount)
		c.compile(node.FuncBody, optimize)
		if c.isLastIns(code.OpPop) {
			c.replaceLast(code.OpReturnVal)
		}
		if !c.isLastIns(code.OpReturnVal) {
			c.emit(code.OpNull)
			c.emit(code.OpReturnVal)
		}
		numLocals := c.symTable.NumDefinitions()
		instructions := c.leaveScope()

		compiledFn := object.CompiledFunc{
			FnName:        node.Name,
			Instructions:  instructions,
			LocalsNum:     numLocals,
			ParametersNum: paramsCount,
			LineLoc:       node.Token.Loc.Line,
		}
		err := c.constants.AddFunc(fnIdx, compiledFn)
		if err != nil {
			c.Push(err)
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
		c.NewErrorF("unknown ast type %s.", reflect.TypeOf(node).String())
	}
}

func (c *Compiler) Compile(node ast.Node) {
	c.compile(node, true)
	c.handleNoCall()
	//need optimize
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

func (c *Compiler) emit(op code.Opcode, operand ...int) int {
	ins := code.Make(op, operand...)
	pos := len(c.curInstruction())
	c.scope[c.scopeIdx].instructions = append(c.curInstruction(), ins...)
	c.setLastIns(op, pos)
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
	ct := c.symTable
	for ct.Outer != nil {
		ct = ct.Outer
	}
	byCode := &bytecode.Bytecode{
		Instruction: c.curInstruction(),
		Constants:   c.constants.Store,
		Symbols:     ct,
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
	c.scope[c.scopeIdx].lastIns = c.curScope().lastIns
}

func (c *Compiler) replaceIns(offset int, newIns []byte) {
	for i := 0; i < len(newIns); i++ {
		c.scope[c.scopeIdx].instructions[offset+i] = newIns[i]
	}
}

func (c *Compiler) changeOperand(opPos int, operand int) {
	op := code.Opcode(c.curInstruction()[opPos])
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
func (c *Compiler) replaceLast(target code.Opcode) {
	lastPos := c.scope[c.scopeIdx].lastIns.offset
	c.replaceIns(lastPos, code.Make(target))
	c.scope[c.scopeIdx].lastIns.op = target
}

func (c *Compiler) getScope(s parser.Symbol, optimize bool) {
	switch s.ScopeType {
	case parser.Global:
		if optimize && c.isLastIns(code.OpSetGlobal) &&
			code.ReadUint16(c.scope[c.scopeIdx].instructions[len(c.curInstruction())-2:]) == uint16(s.Id) {
			opIdx := c.scope[c.scopeIdx].lastIns.offset
			c.scope[c.scopeIdx].instructions[opIdx] = byte(code.OpUpdateGlobal)
		} else {
			c.emit(code.OpGetGlobal, s.Id)
		}
	case parser.Local:
		if optimize && c.isLastIns(code.OpSetLocal) &&
			code.ReadUint16(c.scope[c.scopeIdx].instructions[len(c.curInstruction())-2:]) == uint16(s.Id) {
			opIdx := c.scope[c.scopeIdx].lastIns.offset
			c.scope[c.scopeIdx].instructions[opIdx] = byte(code.OpUpdateLocal)
		} else {
			c.emit(code.OpGetLocal, s.Id)
		}
	case parser.BuiltIn:
		c.emit(code.OpGetBuiltin, s.Id)
	}
}

func (c *Compiler) setScope(s parser.Symbol) {
	switch s.ScopeType {
	case parser.Global:
		c.emit(code.OpSetGlobal, s.Id)
	case parser.Local:
		c.emit(code.OpSetLocal, s.Id)
	}
}

type EmittedIns struct {
	op     code.Opcode
	offset int
}

type CompilationScope struct {
	instructions code.Instructions
	lastIns,
	prevIns EmittedIns
}
