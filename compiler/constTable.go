package compiler

import (
	"Interpreter/object"
	"fmt"
	"unsafe"
)

type ConstTable struct {
	Store []*object.BaseObject
	Num   int
}

func NewConstTable() *ConstTable {
	return &ConstTable{
		Store: make([]*object.BaseObject, 0, 10),
		Num:   0,
	}
}
func (ct *ConstTable) RegFunc(fnName string, paramsNum int) int {
	fn := &object.CFunc{
		Type:          object.CompiledFuncObj,
		FnName:        fnName,
		ParametersNum: paramsNum,
	}
	ct.Store = append(ct.Store, (*object.BaseObject)(unsafe.Pointer(fn)))
	idx := ct.Num
	ct.Num++
	return idx
}

func (ct *ConstTable) AddFunc(idx int, CompiledFn *object.CFunc) error {
	funcName := CompiledFn.FnName
	if funcName != CompiledFn.FnName {
		return fmt.Errorf("func name %s has been defined", funcName)
	}
	ct.Store[idx] = (*object.BaseObject)(unsafe.Pointer(CompiledFn))
	return nil
}

func (ct *ConstTable) AddObj(obj *object.BaseObject) int {
	ct.Store = append(ct.Store, obj)
	idx := ct.Num
	ct.Num++
	return idx
}

func (ct *ConstTable) Find(FnName string) (int, bool) {
	for i, obj := range ct.Store {
		if obj.Type == object.CompiledFuncObj {
			rev := (*object.CFunc)(unsafe.Pointer(obj))
			if rev.FnName == FnName {
				return i, true
			}
		}
	}
	return -1, false
}
