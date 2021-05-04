package main

import (
	"Interpreter/ast"
	"Interpreter/object"
	"Interpreter/tokens"
	"math"
	"reflect"
)

type Interpreter struct {
	*Errors
}

func NewExe() *Interpreter {
	return &Interpreter{
		Errors: NewErr(),
	}
}

func (i *Interpreter) visit(node ast.Node) object.Object {
	switch node := node.(type) {
	case ast.NumberNode:
		return i.visitNum(node)
	case ast.PrefixExpr:
		right := i.visit(node.Right)
		return i.visitPrefix(node, right)
	case ast.InfixExpr:
		left := i.visit(node.Left)
		right := i.visit(node.Right)
		return i.visitInfix(node, left, right)
	}
	i.NewErrorF("Unknown AST type %s", reflect.TypeOf(node).Name())
	return nil
}

func (i *Interpreter) visitNum(node ast.NumberNode) object.Object {
	return &object.Number{
		Value: node.Value,
	}
}

func (i *Interpreter) visitPrefix(node ast.PrefixExpr, right object.Object) object.Object {
	switch right.Type() {
	case object.NumberObj:
		return numberPrefix(node, right)
	}
	i.NewErrorF("Unknown AST type %s", right.Type())
	return nil
}

func (i *Interpreter) visitInfix(node ast.InfixExpr, left, right object.Object) object.Object {
	switch {
	case left.Type() == object.NumberObj && right.Type() == object.NumberObj:
		return numberInfix(node, left, right)
	}
	panic("Type error")
}

func numberPrefix(node ast.PrefixExpr, right object.Object) object.Object {
	rightVal := right.(*object.Number)
	switch node.Op.Type {
	case tokens.Minus:
		return &object.Number{Value: -rightVal.Value}
	case tokens.Plus:
		return &object.Number{Value: rightVal.Value}
	}
	panic("unknown op")
}

func numberInfix(node ast.InfixExpr, left, right object.Object) object.Object {
	leftVal := left.(*object.Number)
	rightVal := right.(*object.Number)
	switch node.Op.Type {
	case tokens.Plus:
		return &object.Number{Value: leftVal.Value + rightVal.Value}
	case tokens.Minus:
		return &object.Number{Value: leftVal.Value - rightVal.Value}
	case tokens.Mul:
		return &object.Number{Value: leftVal.Value * rightVal.Value}
	case tokens.Div:
		return &object.Number{Value: leftVal.Value / rightVal.Value}
	case tokens.Floor:
		return &object.Number{Value: math.Floor(leftVal.Value / rightVal.Value)}
	case tokens.Pow:
		return &object.Number{Value: math.Pow(leftVal.Value, rightVal.Value)}
	}
	panic("unknown op")
}

func Exec(ast ast.Node) object.Object {
	inter := NewExe()
	return inter.visit(ast)
}
