package main

import (
	"Interpreter/code"
	"Interpreter/object"
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
		instructions: code.Instructions{},
		constants:    []object.Object{},
		Errors:       NewErr(),
	}
}

func (c *Compiler) Compile(node Node) {
	switch node := node.(type) {
	case NumberNode:
		numObj := object.Number{Value: node.Value}
		operand := c.addConstant(numObj)
		c.emit(code.OpConstant, operand)
	case PrefixExpr:
		c.Compile(node.Right)
		switch node.Op.Type {
		case Plus:
			c.emit(code.OpPlus)
		case Minus:
			c.emit(code.OpMinus)
		default:
			c.NewErrorF("unknown operator %s", node.Op.Str())
		}
	case InfixExpr:
		c.Compile(node.Left)
		c.Compile(node.Right)
		switch node.Op.Type {
		case Plus:
			c.emit(code.OpAdd)
		case Minus:
			c.emit(code.OpSub)
		case Mul:
			c.emit(code.OpMul)
		case Div:
			c.emit(code.OpDiv)
		default:
			c.NewErrorF("unknown operator %s", node.Op.Str())
		}
	}
}

func (c *Compiler) addConstant(obj object.Object) int {
	c.constants = append(c.constants, obj)
	return len(c.constants) - 1
}

func (c *Compiler) emit(op code.Opcode, operand ...int) {
	ins := code.Make(op, operand...)
	pos := len(c.instructions)
	c.instructions = append(c.instructions, ins...)
	c.setLastIns(op, pos)
}

func (c *Compiler) setLastIns(op code.Opcode, pos int) {
	prevIns := c.lastIns
	last := InsFlag{op: op, offset: pos}
	c.prevIns = prevIns
	c.lastIns = last
}

func (c *Compiler) ByteCode() *Bytecode {
	return &Bytecode{
		Instruction: c.instructions,
		Constants:   c.constants,
	}
}

type Bytecode struct {
	Instruction code.Instructions
	Constants   []object.Object
}

type InsFlag struct {
	op     code.Opcode
	offset int
}
