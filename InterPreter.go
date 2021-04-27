package main

import (
	"Interpreter/object"
	"math"
)

type Interpreter struct {
}

func New() *Interpreter {
	return &Interpreter{}
}

func (i *Interpreter) visit(node Node) object.Object {
	switch node := node.(type) {
	case NumberNode:
		return i.visitNum(node)
	case PrefixExpr:
		right := i.visit(node.Right)
		return i.visitPrefix(node, right)
	case InfixExpr:
		left := i.visit(node.Left)
		right := i.visit(node.Right)
		return i.visitInfix(node, left, right)
	}
	return nil
}

func (i *Interpreter) visitNum(node NumberNode) object.Object {
	return &object.Number{
		Value: node.Value,
	}
}

func (i *Interpreter) visitPrefix(node PrefixExpr, right object.Object) object.Object {
	switch right.Type() {
	case object.NumberObj:
		return numberPrefix(node, right)
	}
	return nil
}

func (i *Interpreter) visitInfix(node InfixExpr, left, right object.Object) object.Object {
	switch {
	case left.Type() == object.NumberObj && right.Type() == object.NumberObj:
		return numberInfix(node, left, right)
	}
	panic("Type error")
}

func numberPrefix(node PrefixExpr, right object.Object) object.Object {
	rightVal := right.(*object.Number)
	switch node.Op.Type {
	case Minus:
		return &object.Number{Value: -rightVal.Value}
	case Plus:
		return &object.Number{Value: rightVal.Value}
	}
	panic("unknown op")
}

func numberInfix(node InfixExpr, left, right object.Object) object.Object {
	leftVal := left.(*object.Number)
	rightVal := right.(*object.Number)
	switch node.Op.Type {
	case Plus:
		return &object.Number{Value: leftVal.Value + rightVal.Value}
	case Minus:
		return &object.Number{Value: leftVal.Value - rightVal.Value}
	case Mul:
		return &object.Number{Value: leftVal.Value * rightVal.Value}
	case Div:
		return &object.Number{Value: leftVal.Value / rightVal.Value}
	case Floor:
		return &object.Number{Value: math.Floor(leftVal.Value / rightVal.Value)}
	case Pow:
		return &object.Number{Value: math.Pow(leftVal.Value, rightVal.Value)}
	}
	panic("unknown op")
}

func Exec(ast Node) object.Object {
	inter := New()
	return inter.visit(ast)
}
