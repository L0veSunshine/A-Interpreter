package compiler

import (
	"Interpreter/object"
	"fmt"
)

type FuncTable struct {
	store   []object.Object
	funcNum int
}

func NewFuncTable() *FuncTable {
	return &FuncTable{
		store:   make([]object.Object, 0, 10),
		funcNum: 0,
	}
}
func (ft *FuncTable) regName(fnName string, paramsNum int) int {
	fn := object.CompiledFunc{
		FnName:        fnName,
		ParametersNum: paramsNum,
	}
	ft.store = append(ft.store, fn)
	idx := ft.funcNum
	ft.funcNum++
	return idx
}

func (ft *FuncTable) addFunc(idx int, CompiledFn object.CompiledFunc) error {
	funcName := CompiledFn.FnName
	if funcName != CompiledFn.FnName {
		return fmt.Errorf("func name %s has been defined", funcName)
	}
	ft.store[idx] = CompiledFn
	return nil
}

func (ft *FuncTable) find(funcName string) int {
	init := -1
	var fnObj object.CompiledFunc
	for idx, fn := range ft.store {
		fnObj = fn.(object.CompiledFunc)
		if fnObj.FnName == funcName {
			init = idx
			break
		}
	}
	return init
}
