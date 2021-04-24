package main

import "fmt"

type AST interface {
	ToString() string
}

type BinOp struct {
	Left, Right AST
	op          Token
}

type UnaryOp struct {
	Expr AST
	op   Token
}

type Num struct {
	token Token
	value float64
}

type Vars struct {
	token Token
	value interface{}
}

type AssignOp struct {
	BinOp
}

type program struct {
	name  string
	block AST
}

type Block struct {
	declarations      []AST
	compoundStatement AST
}

func (b Block) ToString() string {
	var s string
	s = "Declarations "
	for _, d := range b.declarations {
		//for _, inner := range d {
		//	s += inner.ToString() + " "
		//}
		s += d.ToString()
	}
	s += " compoundStatement " + b.compoundStatement.ToString()
	return s
}

type VaeDecl struct {
	varNode, typeNode AST
}

type Type struct {
	token Token
	value interface{}
}

type Compound struct {
	children []AST
}

type NoOp struct {
}

func (vd VaeDecl) ToString() string {
	return vd.varNode.ToString() + vd.typeNode.ToString()
}

func (t Type) ToString() string {
	return "Type " + t.token.Type + " " + fmt.Sprint(t.value)
}

func (o BinOp) ToString() string {
	return fmt.Sprintf("BinOp: [%s %s %s]",
		o.Left.ToString(),
		o.op.Value,
		o.Right.ToString())
}

func (n Num) ToString() string {
	s := fmt.Sprint(n.value)
	return "Num:" + s
}

func (o UnaryOp) ToString() string {
	return fmt.Sprintf("UnaryOp: [%s %s]",
		o.op.Value, o.Expr.ToString())
}

func (co Compound) ToString() string {
	var s string
	for _, c := range co.children {
		s += "{" + c.ToString() + "}"
	}
	return s
}

func (v Vars) ToString() string {
	value := fmt.Sprint(v.value)
	return "(Var " + value + ") "
}

func (o NoOp) ToString() string {
	return ""
}

func (p program) ToString() string {
	return "Program " + p.name + " " + p.block.ToString()
}

func NNum(token Token) Num {
	n := Num{
		token: token,
	}
	value, ok := token.Value.(int)
	if ok {
		n.value = float64(value)
	} else {
		n.value = token.Value.(float64)
	}
	return n
}
