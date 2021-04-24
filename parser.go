package main

import (
	"fmt"
)

type Parser struct {
	lex      *Lexer
	curToken *Token
	err      error
}

func NewParser(lex *Lexer) *Parser {
	return &Parser{
		lex:      lex,
		curToken: lex.NextToken(),
	}
}

func (p *Parser) error() {
	panic("interpreter error")
}

func (p *Parser) eat(Type string) {
	if p.curToken.Type == Type {
		p.curToken = p.lex.NextToken()
		return
	}
	fmt.Println("panic at " + fmt.Sprint(p.curToken.Value))
	panic("panic at " + fmt.Sprint(p.curToken.Value))
}
func (p *Parser) program() AST {
	p.eat(Program)
	node := p.variable()
	programName := node.(Vars).ToString()
	p.eat(Semi)
	blockNode := p.block()
	programNode := program{
		name:  programName,
		block: blockNode,
	}
	p.eat(Dot)
	return programNode
}

func (p *Parser) block() AST {
	declarationNodes := p.declarations()
	compoundStatementNode := p.compoundStatement()
	return Block{
		declarations:      declarationNodes,
		compoundStatement: compoundStatementNode,
	}
}

func (p *Parser) declarations() []AST {
	var declarations []AST
	if p.curToken.Type == Var {
		p.eat(Var)
		for p.curToken.Type == ID {
			varDec1 := p.variableDeclaration()
			declarations = append(declarations, varDec1...)
			p.eat(Semi)
		}
	}
	return declarations
}

func (p *Parser) variableDeclaration() []AST {
	var nodes, varDeclarations []AST
	nodes = append(nodes, Vars{
		token: *p.curToken,
		value: p.curToken.Value,
	})
	p.eat(ID)
	for p.curToken.Type == Comma {
		p.eat(Comma)
		nodes = append(nodes, Vars{token: *p.curToken, value: p.curToken.Value})
		p.eat(ID)
	}
	p.eat(Colon)
	typeNode := p.typeSpec()
	for _, n := range nodes {
		varDeclarations = append(varDeclarations, VaeDecl{varNode: n, typeNode: typeNode})
	}
	return varDeclarations
}

func (p *Parser) typeSpec() AST {
	token := p.curToken
	if p.curToken.Type == Integer {
		p.eat(Integer)
	} else {
		p.eat(Real)
	}
	node := Type{
		token: *token,
		value: token.Value,
	}
	return node
}

func (p *Parser) compoundStatement() AST {
	p.eat(Begin)
	nodes := p.statementList()
	p.eat(End)
	return Compound{children: nodes}
}

func (p *Parser) statementList() (res []AST) {
	var node AST
	node = p.statement()
	res = append(res, node)
	for p.curToken.Type == Semi {
		p.eat(Semi)
		res = append(res, p.statement())
	}
	if p.curToken.Type == ID {
		p.error()
	}
	return res
}

func (p *Parser) statement() AST {
	var node AST
	switch p.curToken.Type {
	case Begin:
		node = p.compoundStatement()
	case ID:
		node = p.assignmentStatement()
	default:
		node = p.empty()
	}
	return node
}

func (p *Parser) assignmentStatement() AST {
	left := p.variable()
	token := p.curToken
	p.eat(Assign)
	right := p.expr()
	return AssignOp{
		BinOp{
			Left:  left,
			Right: right,
			op: Token{
				Type:  token.Type,
				Value: token.Value,
			},
		},
	}
}

func (p *Parser) variable() AST {
	node := Vars{
		token: *p.curToken,
		value: p.curToken.Value,
	}
	p.eat(ID)
	return node
}

func (p *Parser) empty() AST {
	return NoOp{}
}

func (p *Parser) factor() AST {
	token := p.curToken
	switch token.Type {
	case Plus:
		p.eat(Plus)
		return UnaryOp{
			Expr: p.factor(),
			op:   *token,
		}
	case Minus:
		p.eat(Minus)
		return UnaryOp{
			Expr: p.factor(),
			op:   *token,
		}
	case IntegerConst:
		p.eat(IntegerConst)
		return NNum(*token)
	case RealConst:
		p.eat(RealConst)
		return NNum(*token)
	case LParen:
		p.eat(LParen)
		val := p.expr()
		p.eat(RParen)
		return val
	default:
		node := p.variable()
		return node
	}
	//fmt.Println("Invalid factor " + token.Value.(string))
	//panic("Invalid factor")
}

func (p *Parser) term() AST {
	var node AST
	node = p.midFactor()
	for p.curToken.Type == Mul || p.curToken.Type == FloatDiv ||
		p.curToken.Type == IntegerDiv {
		token := *p.curToken
		if p.curToken.Type == Mul {
			p.eat(Mul)
		} else if p.curToken.Type == FloatDiv {
			p.eat(FloatDiv)
		} else if p.curToken.Type == IntegerDiv {
			p.eat(IntegerDiv)
		}
		node = BinOp{
			Left:  node,
			Right: p.midFactor(),
			op:    token,
		}
	}
	return node
}

func (p *Parser) midFactor() AST {
	var node AST
	node = p.factor()
	for p.curToken.Type == Pow {
		token := *p.curToken
		if p.curToken.Type == Pow {
			p.eat(Pow)
		}
		node = BinOp{
			Left:  node,
			Right: p.factor(),
			op:    token,
		}
	}
	return node
}

func (p *Parser) expr() AST {
	var node AST
	node = p.term()
	for p.curToken.Type == Plus || p.curToken.Type == Minus {
		token := *p.curToken
		if p.curToken.Type == Plus {
			p.eat(Plus)
		} else if p.curToken.Type == Minus {
			p.eat(Minus)
		}
		node = BinOp{
			Left:  node,
			op:    token,
			Right: p.term(),
		}
	}
	return node
}

func (p *Parser) Parse() (node AST) {
	node = p.program()
	if p.curToken.Type != EOF {
		panic("Is not EOF")
	}
	return
}
