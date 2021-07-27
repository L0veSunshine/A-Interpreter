package compiler

import (
	"Interpreter/object"
	"fmt"
)

type ConstTable struct {
	Store []object.Object
	Num   int
}

func NewConstTable() *ConstTable {
	return &ConstTable{
		Store: make([]object.Object, 0, 10),
		Num:   0,
	}
}
func (ct *ConstTable) RegFunc(fnName string, paramsNum int) int {
	fn := object.CompiledFunc{
		FnName:        fnName,
		ParametersNum: paramsNum,
	}
	ct.Store = append(ct.Store, fn)
	idx := ct.Num
	ct.Num++
	return idx
}

func (ct *ConstTable) AddFunc(idx int, CompiledFn object.CompiledFunc) error {
	funcName := CompiledFn.FnName
	if funcName != CompiledFn.FnName {
		return fmt.Errorf("func name %s has been defined", funcName)
	}
	ct.Store[idx] = CompiledFn
	return nil
}

func (ct *ConstTable) AddObj(obj object.Object) int {
	ct.Store = append(ct.Store, obj)
	idx := ct.Num
	ct.Num++
	return idx
}

func (ct *ConstTable) Find(FnName string) (int, bool) {
	for i, obj := range ct.Store {
		fn, ok := obj.(object.CompiledFunc)
		if ok {
			if fn.FnName == FnName {
				return i, true
			}
		}
	}
	return -1, false
}
