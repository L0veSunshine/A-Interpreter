package main

import (
	"errors"
	"fmt"
)

type Errors struct {
	errs []error
}

func NewErr() *Errors {
	return &Errors{errs: []error{}}
}

func (e *Errors) NewErrorF(format string, args ...interface{}) {
	errMsg := fmt.Sprintf(format, args...)
	err := errors.New(errMsg)
	e.errs = append(e.errs, err)
}

func (e *Errors) NewError(errMsg string) {
	err := errors.New(errMsg)
	e.errs = append(e.errs, err)
}

func (e *Errors) Push(err error) {
	e.errs = append(e.errs, err)
}

func (e *Errors) HasError() bool {
	return len(e.errs) != 0
}

func (e *Errors) Errors() []error {
	return e.errs
}
