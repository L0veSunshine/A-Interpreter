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
	// ScopeID 是定义该符号的作用域编号。函数符号用它指向自己的函数作用域，
	// 这样同名函数在不同作用域里能各自定位，不会互相覆盖。
	ScopeID int
	// FuncID 是该局部变量所属的函数作用域编号（顶层为 0）。
	// 编译器用它定位捕获变量所属的帧。
	FuncID int
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

// Names 返回已登记的方法名，下标即 OpCallMethod 的方法名操作数。
// 返回的切片是内部存储，调用方只读。
func (n *MethodNames) Names() []string {
	return n.methodName
}

type SymTable struct {
	Outer          *SymTable
	Inner          []*SymTable
	BlockName      string
	ID             int
	store          map[string]Symbol
	numDefinitions int
	// localSlots 是函数级共享的下一个局部槽位号。同一个函数里的所有块
	// （函数体、if/for 分支、嵌套块）共用它，从而保证每个 var 声明拿到
	// 互不冲突的槽位；块之间的槽位不复用，块退出时也就不需要生成清理代码。
	localSlots *int
	// funcID 是所属函数作用域的编号（顶层为 0）。函数表记录自己的编号，
	// 块作用域从父表继承。
	funcID  int
	Methods *MethodNames
}

func NewSymTable(name string) *SymTable {
	return &SymTable{
		Outer:          nil,
		Inner:          nil,
		BlockName:      name,
		store:          map[string]Symbol{},
		numDefinitions: 0,
		localSlots:     new(int),
		Methods:        NewMethodName(),
	}
}
func (st *SymTable) NumDefinitions() int {
	return st.numDefinitions
}

// LocalSlots 返回该作用域所属函数已经分配掉的局部槽位总数，
// 也就是 VM 里帧需要预留的变量数组长度。
func (st *SymTable) LocalSlots() int {
	if st.localSlots == nil {
		return 0
	}
	return *st.localSlots
}

func (st *SymTable) Define(name string, t SymType) Symbol {
	s := Symbol{Name: name, Type: t, ScopeID: st.ID, FuncID: st.funcID}
	if st.Outer == nil {
		s.ScopeType = Global
		s.Id = st.numDefinitions
	} else {
		s.ScopeType = Local
		s.Id = *st.localSlots
		*st.localSlots++
	}
	st.store[name] = s
	st.numDefinitions++
	return s
}

// DefineFunc 登记一个函数名，funcScope 是该函数自己的作用域编号。
// 局部函数占用槽位，保存每次执行定义时创建的闭包实例。
func (st *SymTable) DefineFunc(name string, funcScope int) Symbol {
	s := Symbol{
		Name:      name,
		Type:      F,
		ScopeType: Local,
		ScopeID:   funcScope,
		FuncID:    st.funcID,
	}
	if st.Outer == nil {
		s.ScopeType = Global
		s.Id = st.numDefinitions
	} else {
		s.Id = st.AllocLocal()
	}
	st.store[name] = s
	st.numDefinitions++
	return s
}

// AllocLocal 为当前函数分配一个匿名局部槽位（编译器内部用的临时量）。
// 返回的槽位只被生成的字节码引用，不映射到任何源码变量名。
func (st *SymTable) AllocLocal() int {
	if st.localSlots == nil {
		st.localSlots = new(int)
	}
	id := *st.localSlots
	*st.localSlots++
	return id
}

// Defined 只检查当前作用域，用于发现同一块里的重复声明。
func (st *SymTable) Defined(name string) bool {
	_, ok := st.store[name]
	return ok
}

func (st *SymTable) Root() *SymTable {
	cur := st
	for cur.Outer != nil {
		cur = cur.Outer
	}
	return cur
}

// FindScope 按编译期作用域编号定位符号表（包括表和函数表），
// 让编译器在遍历 AST 时能与解析期建立的作用域一一对应。
func (st *SymTable) FindScope(id int) *SymTable {
	root := st.Root()
	if root.ID == id {
		return root
	}
	queue := []*SymTable{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, in := range cur.Inner {
			if in.ID == id {
				return in
			}
			queue = append(queue, in)
		}
	}
	return nil
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

// FindLocalByIdx / FindGlobalByIdx 分开按作用域查找。
// 局部槽位和全局槽位各自从 0 编号，混在一起找会把同号的另一个变量名显示出来。
func (st *SymTable) FindLocalByIdx(index int) (string, bool) {
	return st.findByIdxInScope(index, Local)
}

func (st *SymTable) FindGlobalByIdx(index int) (string, bool) {
	return st.findByIdxInScope(index, Global)
}

func (st *SymTable) findByIdxInScope(index int, scope Scope) (string, bool) {
	for _, s := range st.store {
		if s.Id == index && s.ScopeType == scope {
			return s.Name, true
		}
	}
	for _, in := range st.Inner {
		if name, ok := in.findByIdxInScope(index, scope); ok {
			return name, true
		}
	}
	return "", false
}

func (st *SymTable) FindByIdx(index int) (string, bool) {
	for _, s := range st.store {
		if s.Id == index && s.ScopeType != BuiltIn {
			return s.Name, true
		}
	}
	// 块级作用域里的变量挂在子表上，反汇编时需要一起搜索
	for _, in := range st.Inner {
		if name, ok := in.FindByIdx(index); ok {
			return name, true
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
	// 块作用域：和所属函数共用槽位计数器，并继承所属函数的编号
	table.localSlots = enter.localSlots
	if table.localSlots == nil {
		table.localSlots = new(int)
	}
	table.funcID = enter.funcID
	return table
}

// NewFuncSymTable 建立函数作用域：局部槽位从 0 重新开始，
// 因为每次调用都会按 LocalsNum 新建一个帧。
// funcID 是该函数作用域自身的编号，由调用方分配后写入。
func NewFuncSymTable(name string, enter *SymTable, funcID int) *SymTable {
	table := NewSymTable(name)
	enter.Inner = append(enter.Inner, table)
	table.Outer = enter
	table.Methods = enter.Methods
	table.localSlots = new(int)
	table.ID = funcID
	table.funcID = funcID
	return table
}
