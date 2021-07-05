package compiler

import (
	"Interpreter/object"
	"fmt"
)

type FuncTable struct {
	store   map[string]object.CompiledFunc
	funcNum int
}

func NewFuncTable() *FuncTable {
	return &FuncTable{
		store:   make(map[string]object.CompiledFunc, 10),
		funcNum: 0,
	}
}

func (ft *FuncTable) addFunc(CompiledFn object.CompiledFunc) error {
	funcName := CompiledFn.FnName
	_, ok := ft.store[funcName]
	if ok {
		return fmt.Errorf("func name %s has been defined", funcName)
	}
	ft.store[funcName] = CompiledFn
	ft.funcNum++
	return nil
}

func (ft *FuncTable) find(funcName string) (object.CompiledFunc, bool) {
	fn, ok := ft.store[funcName]
	if !ok {
		return object.CompiledFunc{}, ok
	}
	fn.Called = true
	return fn, true
}
