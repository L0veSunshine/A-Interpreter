package bytecode

type SymbolScope string

const (
	GlobalScope  SymbolScope = "GLOBAL"
	LocalScope   SymbolScope = "LOCAL"
	BuiltinScope SymbolScope = "BUILTIN"
	FreeScope    SymbolScope = "FREE"
)

type Symbol struct {
	Name  string
	Scope SymbolScope
	Index int
}

type SymbolTable struct {
	Outer, Inner   *SymbolTable
	store          map[string]Symbol
	numDefinitions int
	FreeSymbols    []Symbol
}

func (st *SymbolTable) NumDefinitions() int {
	return st.numDefinitions
}

func (st *SymbolTable) Define(name string) Symbol {
	symbol := Symbol{
		Name:  name,
		Index: st.numDefinitions,
	}
	if st.Outer == nil {
		symbol.Scope = GlobalScope
	} else {
		symbol.Scope = LocalScope
	}
	st.store[name] = symbol
	st.numDefinitions++
	return symbol
}

func (st *SymbolTable) Resolve(name string) (Symbol, bool) {
	symbol, ok := st.store[name]
	if !ok && st.Outer != nil {
		symbol, ok := st.Outer.Resolve(name)
		if !ok {
			return symbol, ok
		}
		if symbol.Scope == GlobalScope || symbol.Scope == BuiltinScope {
			return symbol, ok
		}
		free := st.defineFree(symbol)
		return free, true
	}
	return symbol, ok
}

func (st *SymbolTable) defineFree(origin Symbol) Symbol {
	st.FreeSymbols = append(st.FreeSymbols, origin)
	symbol := Symbol{
		Name:  origin.Name,
		Scope: FreeScope,
		Index: len(st.FreeSymbols) - 1,
	}
	st.store[origin.Name] = symbol
	return symbol
}

func (st *SymbolTable) DefineBuiltin(index int, name string) Symbol {
	symbol := Symbol{
		Name:  name,
		Index: index,
		Scope: BuiltinScope,
	}
	st.store[name] = symbol
	return symbol
}

func (st *SymbolTable) findByIndex(idx int) (string, bool) {
	var res string
	for name, symbol := range st.store {
		if symbol.Index == idx {
			res = name
		}
	}
	if res == "" {
		if st.Outer == nil {
			return res, false
		} else {
			return st.Outer.findByIndex(idx)
		}
	}
	return res, true
}

func NewSymbolTable() *SymbolTable {
	var frees []Symbol
	return &SymbolTable{
		store:          map[string]Symbol{},
		numDefinitions: 0,
		FreeSymbols:    frees,
	}
}

func NewEnclosedSymbolTable(outer *SymbolTable) *SymbolTable {
	s := NewSymbolTable()
	outer.Inner = s
	s.Outer = outer
	return s
}
