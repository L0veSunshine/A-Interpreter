package main

type SymbolScope string

const (
	GlobalScope SymbolScope = "GLOBAL"
)

type Symbol struct {
	Name  string
	Scope SymbolScope
	Index int
}

type SymbolTable struct {
	store          map[string]Symbol
	numDefinitions int
}

func (st *SymbolTable) Define(name string) Symbol {
	symbol := Symbol{
		Name:  name,
		Scope: GlobalScope,
		Index: st.numDefinitions,
	}
	st.store[name] = symbol
	st.numDefinitions += 1
	return symbol
}

func (st *SymbolTable) Resolve(name string) (Symbol, bool) {
	symbol, ok := st.store[name]
	return symbol, ok
}

func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		store:          map[string]Symbol{},
		numDefinitions: 0,
	}
}
