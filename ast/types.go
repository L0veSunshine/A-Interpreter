package ast

type Node interface {
	TokenLiteral() string
	Str() string
	// Line 返回该节点对应的源码行号（0 表示未知）。
	// 编译器用它生成"字节码偏移 -> 源码行"的映射，让运行时错误能定位到行。
	Line() int
}

type Expression interface {
	Node
	expressionNode()
}

type Statement interface {
	Node
	StatementNode()
}

// str 是 AST 打印用的空值安全取值：语法错误时 AST 里可能出现 nil 子节点，
// 打印/调试路径不应该因此崩溃。
func str(n Node) string {
	if n == nil {
		return ""
	}
	return n.Str()
}

// nodeLine 是空值安全的行号取值。
func nodeLine(n Node) int {
	if n == nil {
		return 0
	}
	return n.Line()
}

// firstStatementLine 让 Program 的行号取第一条语句的行号。
func firstStatementLine(p Program) int {
	for _, s := range p.Statements {
		if l := nodeLine(s); l > 0 {
			return l
		}
	}
	return 0
}
