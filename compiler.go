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
		instructions: code.Instructions{},
		constants:    []object.Object{},
		Errors:       NewErr(),
	}
}

func (c *Compiler) Compile(node ast.Node) {
	switch node := node.(type) {
	case ast.Program:
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
		boolObj := object.Boolean{Value: node.Value}
		consIdx := c.addConstant(boolObj)
		c.emit(code.OpConstant, consIdx)
	case ast.PrefixExpr:
		c.Compile(node.Right)
		switch node.Op.Type {
		case tokens.Plus:
			c.emit(code.OpPlus)
		case tokens.Minus:
			c.emit(code.OpMinus)
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
		case tokens.Not:
			c.emit(code.OpNot)
		default:
			c.NewErrorF("unknown operator %s", node.Op.Str())
		}
	default:
		c.NewErrorF("unknown ast type %s", reflect.TypeOf(node).String())
	}
}

func (c *Compiler) addConstant(obj object.Object) (idx int) {
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
