package bytecode

import (
	"Interpreter/object"
	"fmt"
)

type FuncTable struct {
	Store   []object.Object
	FuncNum int
}

func NewFuncTable() *FuncTable {
	return &FuncTable{
		Store:   make([]object.Object, 0, 10),
		FuncNum: 0,
	}
}
func (ft *FuncTable) RegName(fnName string, paramsNum int) int {
	fn := object.CompiledFunc{
		FnName:        fnName,
		ParametersNum: paramsNum,
	}
	ft.Store = append(ft.Store, fn)
	idx := ft.FuncNum
	ft.FuncNum++
	return idx
}

func (ft *FuncTable) AddFunc(idx int, CompiledFn object.CompiledFunc) error {
	funcName := CompiledFn.FnName
	if funcName != CompiledFn.FnName {
		return fmt.Errorf("func name %s has been defined", funcName)
	}
	ft.Store[idx] = CompiledFn
	return nil
}

func (ft *FuncTable) Find(funcName string) int {
	init := -1
	var fnObj object.CompiledFunc
	for idx, fn := range ft.Store {
		fnObj = fn.(object.CompiledFunc)
		if fnObj.FnName == funcName {
			init = idx
			break
		}
	}
	return init
}
