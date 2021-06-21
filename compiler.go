package main

import (
	"Interpreter/ast"
	"Interpreter/code"
	"Interpreter/object"
	"Interpreter/tokens"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type Compiler struct {
	*Errors

	debug        bool
	instructions code.Instructions
	constants    []object.Object
	symbolTable  *SymbolTable
	lastIns,
	prevIns InsFlag
}

func NewCompiler() *Compiler {
	return &Compiler{
		Errors:       NewErr(),
		debug:        false,
		instructions: code.Instructions{},
		constants:    []object.Object{},
		symbolTable:  NewSymbolTable(),
		lastIns:      InsFlag{},
		prevIns:      InsFlag{},
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
		if len(c.errs) != 0 {
			fmt.Println("ERROR:", c.errs)
			return
		}
	}
	fmt.Print(sb.String())
}

func (c *Compiler) Compile(node ast.Node) {
	switch node := node.(type) {
	case ast.Program:
		for _, s := range node.Statements {
			c.Compile(s)
		}
	case *ast.BlockStatement:
		for _, s := range node.Statements {
			c.Compile(s)
		}
	case ast.ExprStatement:
		c.Compile(node.Expression)
		c.emit(code.OpPop)
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
		c.Compile(node.Right)
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
			c.Compile(node.Right)
			c.Compile(node.Left)
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
		c.Compile(node.Left)
		c.Compile(node.Right)
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
		c.Compile(node.Condition)
		jumpNotTruePos := c.emit(code.OpJumpNotTrue, 9999)
		c.Compile(node.Consequence)
		if c.isLastIns(code.OpPop) {
			c.removeLastOp()
		}
		jumpPos := c.emit(code.OpJump, 9999)
		afterConSeqPos := len(c.instructions)
		c.changeOperand(jumpNotTruePos, afterConSeqPos)
		if node.Alternative == nil {
			c.emit(code.OpNull)
		} else {
			c.Compile(node.Alternative)
			if c.isLastIns(code.OpPop) {
				c.removeLastOp()
			}
		}
		afterAlterPos := len(c.instructions)
		c.changeOperand(jumpPos, afterAlterPos)
	case ast.ForExpression:
		forStatPos := len(c.instructions)
		c.Compile(node.Condition)
		breakPos := c.emit(code.OpJumpNotTrue, 9999)
		c.Compile(node.Loop)
		if c.isLastIns(code.OpPop) {
			c.removeLastOp()
		}
		c.emit(code.OpJump, forStatPos)
		c.changeOperand(breakPos, len(c.instructions))
		c.emit(code.OpNull)
	case ast.VarStatement:
		c.Compile(node.Value)
		symbol := c.symbolTable.Define(node.Indent.Value)
		c.emit(code.OpSetGlobal, symbol.Index)
	case ast.IdentNode:
		symbol, ok := c.symbolTable.Resolve(node.Value)
		if !ok {
			c.NewErrorF("undefined variable %s.", strconv.Quote(node.Value))
		}
		c.emit(code.OpGetGlobal, symbol.Index)
	case ast.AssignStatement:
		c.Compile(node.Statement)
		symbol, ok := c.symbolTable.Resolve(node.Identifier.Value)
		if !ok {
			c.NewErrorF("variable %s is undefined but used.", strconv.Quote(node.Identifier.Value))
		} else {
			c.emit(code.OpUpdate, symbol.Index)
		}
	default:
		c.NewErrorF("unknown ast type %s", reflect.TypeOf(node).String())
	}
}

func (c *Compiler) addConstant(obj object.Object) (idx int) {
	c.constants = append(c.constants, obj)
	return len(c.constants) - 1
}

func (c *Compiler) emit(op code.Opcode, operand ...int) int {
	ins := code.Make(op, operand...)
	pos := len(c.instructions)
	c.instructions = append(c.instructions, ins...)
	c.setLastIns(op, pos)
	return pos
}

func (c *Compiler) setLastIns(op code.Opcode, pos int) {
	prevIns := c.lastIns
	last := InsFlag{op: op, offset: pos}
	c.prevIns = prevIns
	c.lastIns = last
}

func (c *Compiler) ByteCode() *code.Bytecode {
	if c.HasError() {
		return &code.Bytecode{}
	}
	byCode := &code.Bytecode{
		Instruction: c.instructions,
		Constants:   c.constants,
	}
	return byCode
}

func (c *Compiler) isLastIns(op code.Opcode) bool {
	return c.lastIns.op == op
}

func (c *Compiler) removeLastOp() {
	c.instructions = c.instructions[:c.lastIns.offset]
	c.lastIns = c.prevIns
}

func (c *Compiler) replaceIns(offset int, newIns []byte) {
	for i := 0; i < len(newIns); i++ {
		c.instructions[offset+i] = newIns[i]
	}
}

func (c *Compiler) changeOperand(opPos int, operand int) {
	op := code.Opcode(c.instructions[opPos])
	newIns := code.Make(op, operand)
	c.replaceIns(opPos, newIns)
}

type InsFlag struct {
	op     code.Opcode
	offset int
}
