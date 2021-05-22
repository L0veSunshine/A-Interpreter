package main

import (
	"Interpreter/ast"
	"Interpreter/code"
	"Interpreter/object"
	"Interpreter/tokens"
	"reflect"
)

type Compiler struct {
	*Errors

	instructions code.Instructions
	constants    []object.Object
	lastIns,
	prevIns InsFlag
}

func NewCompiler() *Compiler {
	return &Compiler{
		Errors:       NewErr(),
		instructions: code.Instructions{},
		constants:    []object.Object{},
		lastIns:      InsFlag{},
		prevIns:      InsFlag{},
	}
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
	case ast.NumberNode:
		numObj := object.Number{Value: node.Value}
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
		case tokens.Floor:
			c.emit(code.OpFloor)
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
	return &code.Bytecode{
		Instruction: c.instructions,
		Constants:   c.constants,
	}
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
