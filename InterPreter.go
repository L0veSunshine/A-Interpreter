package main

import (
	"fmt"
	"math"
)

type Interpreter struct {
	parser Parser
}

func New() *Interpreter {
	return &Interpreter{}
}

var Global = map[string]interface{}{}

func (i *Interpreter) visit(ast AST) float64 {
	switch ast.(type) {
	case BinOp:
		return i.visitBinOp(ast.(BinOp))
	case Num:
		return i.visitNum(ast.(Num))
	case UnaryOp:
		return i.visitUnaryOp(ast.(UnaryOp))
	case Compound:
		return i.visitCompound(ast.(Compound))
	case NoOp:
		return i.visitNoOp(ast.(NoOp))
	case Vars:
		return i.visitVar(ast.(Vars))
	case AssignOp:
		return i.visitAssign(ast.(AssignOp))
	case program:
		return i.visitProgram(ast.(program))
	case Block:
		return i.visitBlock(ast.(Block))
	case VaeDecl:
		return i.visitVarDecl(ast.(VaeDecl))
	case Type:
		return i.visitType(ast.(Type))
	}
	panic("unknown ast node")
}

func (i *Interpreter) visitBinOp(node BinOp) float64 {
	switch node.op.Type {
	case Plus:
		return i.visit(node.Left) + i.visit(node.Right)
	case Minus:
		return i.visit(node.Left) - i.visit(node.Right)
	case Mul:
		return i.visit(node.Left) * i.visit(node.Right)
	case IntegerDiv:
		return float64(int(i.visit(node.Left)) / int(i.visit(node.Right)))
	case FloatDiv:
		return i.visit(node.Left) / i.visit(node.Right)
	case Pow:
		return math.Pow(i.visit(node.Left), i.visit(node.Right))
	default:
		panic("error")
	}
}
func (i *Interpreter) visitNum(node Num) float64 {
	return node.value
}

func (i *Interpreter) visitUnaryOp(node UnaryOp) float64 {
	if node.op.Type == Minus {
		return -i.visit(node.Expr)
	}
	return i.visit(node.Expr)
}

func (i *Interpreter) visitCompound(node Compound) float64 {
	for _, n := range node.children {
		i.visit(n)
	}
	return 0
}
func (i *Interpreter) visitAssign(node AssignOp) float64 {
	varName := node.Left.(Vars).value.(string)
	Global[varName] = i.visit(node.Right)
	return 0
}

func (i *Interpreter) visitVar(node Vars) float64 {
	varName := node.value.(string)
	if val, ok := Global[varName]; ok {
		return val.(float64)
	} else {
		panic(fmt.Sprintf("unknown variable: %s", varName))
		return 0
	}
}

func (i *Interpreter) visitNoOp(node NoOp) float64 {
	return 0
}

func (i *Interpreter) visitProgram(node program) float64 {
	i.visit(node.block)
	return 0
}
func (i *Interpreter) visitBlock(node Block) float64 {
	for _, d := range node.declarations {
		i.visit(d)
	}
	i.visit(node.compoundStatement)
	return 0
}

func (i *Interpreter) visitVarDecl(node VaeDecl) float64 {
	return 0
}

func (i *Interpreter) visitType(node Type) float64 {
	return 0
}

func (i *Interpreter) GetGlobalTable() map[string]interface{} {
	return Global
}

func Exec(ast AST) float64 {
	inter := New()
	return inter.visit(ast)
}
