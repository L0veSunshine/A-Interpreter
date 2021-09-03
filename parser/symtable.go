package parser

type Scope string
type SymType string

const (
	Global  Scope   = "Global"
	Local   Scope   = "Local"
	BuiltIn Scope   = "BuiltIn"
	I       SymType = "Indent"
	F       SymType = "Func"
)

type Symbol struct {
	Name      string
	Type      SymType
	ScopeType Scope
	Id        int
}

type MethodNames struct {
	Index      int
	methodName []string
}

func NewMethodName() *MethodNames {
	var name []string
	return &MethodNames{
		Index:      -1,
		methodName: name,
	}
}

func (n *MethodNames) Add(name string) {
	n.methodName = append(n.methodName, name)
	n.Index++
}

func (n *MethodNames) FindName(idx int) string {
	return n.methodName[idx]
}

func (n *MethodNames) FindIdx(name string) (int, bool) {
	for i, n := range n.methodName {
		if n == name {
			return i, true
		}
	}
	return -1, false
}

type SymTable struct {
	Outer          *SymTable
	Inner          []*SymTable
	BlockName      string
	store          map[string]Symbol
	numDefinitions int
	Methods        *MethodNames
}

func NewSymTable(name string) *SymTable {
	return &SymTable{
		Outer:          nil,
		Inner:          nil,
		BlockName:      name,
		store:          map[string]Symbol{},
		numDefinitions: 0,
		Methods:        NewMethodName(),
	}
}
func (st *SymTable) NumDefinitions() int {
	return st.numDefinitions
}

func (st *SymTable) Define(name string, t SymType) Symbol {
	s := Symbol{
		Name: name,
		Type: t,
		Id:   st.numDefinitions,
	}
	if st.Outer == nil {
		s.ScopeType = Global
	} else {
		s.ScopeType = Local
	}
	st.store[name] = s
	st.numDefinitions++
	return s
}

func (st *SymTable) DefineBuiltin(name string, index int) Symbol {
	s := Symbol{
		Name:      name,
		Type:      F,
		ScopeType: BuiltIn,
		Id:        index,
	}
	st.store[name] = s
	return s
}

func (st *SymTable) Resolve(name string) (Symbol, bool) {
	s, ok := st.store[name]
	if ok {
		return s, true
	}
	if st.Outer != nil && !ok {
		return st.Outer.Resolve(name)
	}
	return Symbol{}, false
}

func (st *SymTable) FindByIdx(index int) (string, bool) {
	for _, s := range st.store {
		if s.Id == index && s.ScopeType != BuiltIn {
			return s.Name, true
		}
	}
	return "", false
}

func Search(name string, enter *SymTable) *SymTable {
	if enter.BlockName == name {
		return enter
	}
	var cur = enter
	for cur.Outer != nil {
		cur = cur.Outer
	}
	return search(name, cur.Inner)
}

func search(name string, children []*SymTable) *SymTable {
	var inners []*SymTable
	for _, c := range children {
		if c.BlockName == name {
			return c
		} else {
			inners = append(inners, c.Inner...)
		}
	}
	if len(inners) > 0 {
		return search(name, inners)
	} else {
		return nil
	}
}

func NewInnerSymTable(name string, enter *SymTable) *SymTable {
	table := NewSymTable(name)
	enter.Inner = append(enter.Inner, table)
	table.Outer = enter
	table.Methods = enter.Methods
	return table
}
