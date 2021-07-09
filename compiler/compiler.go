package compiler

import (
	"Interpreter/ast"
	"Interpreter/bytecode"
	"Interpreter/code"
	"Interpreter/errors"
	"Interpreter/object"
	"Interpreter/tokens"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type Compiler struct {
	*errors.Errors

	debug       bool
	constants   []object.Object
	symbolTable *SymbolTable
	functions   *FuncTable
	scope       []CompilationScope
	scopeIdx    int
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

	st := NewSymbolTable()
	for k, v := range object.BuiltinFns {
		st.DefineBuiltin(k, v.Name)
	}

	return &Compiler{
		Errors:      errors.NewErr(),
		debug:       false,
		constants:   []object.Object{},
		symbolTable: st,
		functions:   NewFuncTable(),
		scope:       []CompilationScope{rootScope},
		scopeIdx:    0,
	}
}

func (c *Compiler) Debug() {
	ls := strings.Repeat("=", 40) + "\n"
	c.debug = true
	var sb strings.Builder
	if c.debug {
		sb.WriteString(ls)
		sb.WriteString(fmt.Sprintf("%25s\n", "Byte Code"))
		sb.WriteString(ls)
		sb.WriteString(c.ByteCode().String() + "\n")
		sb.WriteString(ls)
		if c.HasError() {
			fmt.Println("ERROR:", c.Errs())
			return
		}
	}
	fmt.Print(sb.String())
}

func (c *Compiler) compile(node ast.Node) {
	switch node := node.(type) {
	case ast.Program:
		for _, s := range node.Statements {
			c.compile(s)
		}
	case *ast.BlockStatement:
		for _, s := range node.Statements {
			c.compile(s)
		}
	case ast.ExprStatement:
		c.compile(node.Expression)
		c.emit(code.OpPop)
	case ast.FuncStatement:
		c.compile(node.Expression)
	case ast.IntNode:
		numObj := object.Int{Value: node.Value}
		consIdx := c.addConstant(numObj)
		c.emit(code.OpConstant, consIdx)
	case ast.FloatNode:
		numObj := object.Float{Value: node.Value}
		consIdx := c.addConstant(numObj)
		c.emit(code.OpConstant, consIdx)
	case ast.StringNode:
		strObj := object.String{Value: node.Value}
		consIdx := c.addConstant(strObj)
		c.emit(code.OpConstant, consIdx)
	case ast.BooleanNode:
		value := node.Value
		if value {
			c.emit(code.OpTrue)
		} else {
			c.emit(code.OpFalse)
		}
	case ast.PrefixExpr:
		c.compile(node.Right)
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
		if node.Op.Type == tokens.LT || node.Op.Type == tokens.LTEq {
			c.compile(node.Right)
			c.compile(node.Left)
			switch node.Op.Type {
			case tokens.LT:
				c.emit(code.OpGT)
			case tokens.LTEq:
				c.emit(code.OpGTEq)
			default:
				c.NewErrorF("unsupported op %s", node.Op.Str())
			}
			return
		}
		c.compile(node.Left)
		c.compile(node.Right)
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
	case ast.IfExpression:
		c.compile(node.Condition)
		jumpNotTruePos := c.emit(code.OpJumpNotTrue, 9999)
		c.compile(node.Consequence)
		if c.isLastIns(code.OpPop) {
			c.removeLastOp()
		}
		jumpPos := c.emit(code.OpJump, 9999)
		afterConSeqPos := len(c.curInstruction())
		c.changeOperand(jumpNotTruePos, afterConSeqPos)
		if node.Alternative == nil {
			c.emit(code.OpNull)
		} else {
			c.compile(node.Alternative)
			if c.isLastIns(code.OpPop) {
				c.removeLastOp()
			}
		}
		afterAlterPos := len(c.curInstruction())
		c.changeOperand(jumpPos, afterAlterPos)
	case ast.ForExpression:
		forStatPos := len(c.curInstruction())
		c.compile(node.Condition)
		breakPos := c.emit(code.OpJumpNotTrue, 9999)
		c.compile(node.Loop)
		if c.isLastIns(code.OpPop) {
			c.removeLastOp()
		}
		c.emit(code.OpJump, forStatPos)
		c.changeOperand(breakPos, len(c.curInstruction()))
		c.emit(code.OpNull)
	case ast.VarStatement:
		c.compile(node.Value)
		symbol := c.symbolTable.Define(node.Indent.Value)
		c.setScope(symbol)
	case ast.IdentNode:
		symbol, ok := c.symbolTable.Resolve(node.Value)
		if !ok {
			fnIdx := c.functions.find(node.Value)
			if fnIdx == -1 {
				c.NewErrorF("undefined variable %s.", strconv.Quote(node.Value))
			}
			c.emit(code.OpClosure, fnIdx)
		}
		c.getScope(symbol)
	case ast.AssignStatement:
		c.compile(node.Statement)
		symbol, ok := c.symbolTable.Resolve(node.Identifier.Value)
		if !ok {
			c.NewErrorF("variable %s is undefined but used.", strconv.Quote(node.Identifier.Value))
		} else {
			c.updateScope(symbol)
		}
	case ast.FuncDef:
		c.enterScope()

		paramsCount := len(node.Parameters)
		fnIdx := c.functions.regName(node.Name, paramsCount)
		for _, p := range node.Parameters {
			c.symbolTable.Define(p.Value)
		}
		c.compile(node.FuncBody)
		if c.isLastIns(code.OpPop) {
			c.replaceLast(code.OpReturnVal)
		}
		if !c.isLastIns(code.OpReturnVal) {
			c.emit(code.OpReturn)
		}
		numLocals := c.symbolTable.numDefinitions
		instructions := c.leaveScope()

		compiledFn := object.CompiledFunc{
			FnName:        node.Name,
			Instructions:  instructions,
			LocalsNum:     numLocals,
			ParametersNum: paramsCount,
		}
		err := c.functions.addFunc(fnIdx, compiledFn)
		if err != nil {
			c.Push(err)
		}
	case ast.ReturnStatement:
		c.compile(node.ReturnVal)
		c.emit(code.OpReturnVal)
	case ast.FuncCallExpr:
		c.compile(node.Function)
		for _, arg := range node.Arguments {
			c.compile(arg)
		}
		c.emit(code.OpCallFunc, len(node.Arguments))
	default:
		c.NewErrorF("unknown ast type %s", reflect.TypeOf(node).String())
	}
}
func (c *Compiler) Compile(node ast.Node) {
	c.compile(node)
	c.handleNoCall()
}

func (c *Compiler) handleNoCall() {
	if c.functions.funcNum == 0 {
		return
	}
	var s = false
	for _, fn := range c.functions.store {
		s = s || fn.Called
	}
	if !s && len(c.curInstruction()) == 0 {
		c.emit(code.OpNull)
		c.emit(code.OpPop)
	}
}

func (c *Compiler) addConstant(obj object.Object) (idx int) {
	c.constants = append(c.constants, obj)
	return len(c.constants) - 1
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
	byCode := &bytecode.Bytecode{
		Instruction: c.curInstruction(),
		Constants:   c.constants,
		Functions:   c.functions.store,
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
	c.symbolTable = NewEnclosedSymbolTable(c.symbolTable)
}

func (c *Compiler) leaveScope() code.Instructions {
	instructions := c.curInstruction()
	c.scope = c.scope[:len(c.scope)-1]
	c.scopeIdx--
	c.symbolTable = c.symbolTable.Outer
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

func (c *Compiler) getScope(s Symbol) {
	switch s.Scope {
	case GlobalScope:
		c.emit(code.OpGetGlobal, s.Index)
	case LocalScope:
		c.emit(code.OpGetLocal, s.Index)
	case BuiltinScope:
		c.emit(code.OpGetBuiltin, s.Index)
	}
}

func (c *Compiler) setScope(s Symbol) {
	switch s.Scope {
	case GlobalScope:
		c.emit(code.OpSetGlobal, s.Index)
	case LocalScope:
		c.emit(code.OpSetLocal, s.Index)
	}
}

func (c *Compiler) updateScope(s Symbol) {
	switch s.Scope {
	case GlobalScope:
		c.emit(code.OpUpdateGlobal, s.Index)
	case LocalScope:
		c.emit(code.OpUpdateLocal, s.Index)
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
