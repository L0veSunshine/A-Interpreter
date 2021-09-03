package object

import (
	"fmt"
	"strings"
)

type String struct {
	Value []rune
}

func (s String) Type() ObjType {
	return StringObj
}

func (s String) Inspect() string {
	return string(s.Value)
}
func stringSplit(self Object, args ...Object) []Object {
	rs := self.(String).Value
	if len(args) != 1 {
		return []Object{Error{ErrorMsg: fmt.Sprintf("want 1 arg but get %d", len(args))}}
	}
	arg := args[0]
	if arg.Type() != StringObj {
		return []Object{Error{ErrorMsg: fmt.Sprintf("TypeError: must be str or None, not %s", arg.Type())}}
	}
	target := arg.(String).Value
	subStr := strings.Split(string(rs), string(target))
	var res []Object
	for _, s := range subStr {
		res = append(res, String{Value: []rune(s)})
	}
	return []Object{self, Array{Elements: res}}
}

func toUpper(self Object, args ...Object) []Object {
	rs := self.(String).Value
	if len(args) > 0 {
		return []Object{Error{ErrorMsg: fmt.Sprintf("want not arg but get %d", len(args))}}
	}
	nrs := navToUpper(rs)
	return []Object{self, String{Value: nrs}}
}

func toLower(self Object, args ...Object) []Object {
	rs := self.(String).Value
	if len(args) > 0 {
		return []Object{Error{ErrorMsg: fmt.Sprintf("want not arg but get %d", len(args))}}
	}
	nrs := navToLower(rs)
	return []Object{self, String{Value: nrs}}
}

func navToUpper(rs []rune) []rune {
	nrs := make([]rune, 0, len(rs))
	for _, r := range rs {
		if 97 <= r && r <= 122 {
			r -= 32
		}
		nrs = append(nrs, r)
	}
	return nrs
}

func navToLower(rs []rune) []rune {
	nrs := make([]rune, 0, len(rs))
	for _, r := range rs {
		if 65 <= r && r <= 90 {
			r += 32
		}
		nrs = append(nrs, r)
	}
	return nrs
}

var StringMethodList = ObjMethods{
	"split": MethodObj{M: stringSplit},
	"upper": MethodObj{M: toUpper},
	"lower": MethodObj{M: toLower},
}
