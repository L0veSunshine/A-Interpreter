package ast

type Node interface {
	TokenLiteral() string
	Str() string
}

type Expression interface {
	Node
	expressionNode()
}

type Statement interface {
	Node
	StatementNode()
}
