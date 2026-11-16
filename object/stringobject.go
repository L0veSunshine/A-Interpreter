package object

import (
	"fmt"
	"strings"

	"Interpreter/format"
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
func stringSplit(self Object, args []Object) (Object, Object, error) {
	s, ok := self.(String)
	if !ok {
		return self, nil, fmt.Errorf(format.Alert+"split() needs a String, got %s", self.Type())
	}
	if len(args) != 1 {
		return self, nil, fmt.Errorf(format.Alert+"want 1 arg but get %d", len(args))
	}
	arg := args[0]
	sep, ok := arg.(String)
	if !ok {
		return self, nil, fmt.Errorf(format.Alert+"TypeError: must be str or None, not %s", arg.Type())
	}
	subStr := strings.Split(string(s.Value), string(sep.Value))
	res := make([]Object, len(subStr))
	for i, part := range subStr {
		res[i] = String{Value: []rune(part)}
	}
	return s, &Array{Elements: res}, nil
}

func toUpper(self Object, args []Object) (Object, Object, error) {
	s, ok := self.(String)
	if !ok {
		return self, nil, fmt.Errorf(format.Alert+"upper() needs a String, got %s", self.Type())
	}
	if len(args) > 0 {
		return self, nil, fmt.Errorf(format.Alert+"want not arg but get %d", len(args))
	}
	return s, String{Value: navToUpper(s.Value)}, nil
}

func toLower(self Object, args []Object) (Object, Object, error) {
	s, ok := self.(String)
	if !ok {
		return self, nil, fmt.Errorf(format.Alert+"lower() needs a String, got %s", self.Type())
	}
	if len(args) > 0 {
		return self, nil, fmt.Errorf(format.Alert+"want not arg but get %d", len(args))
	}
	return s, String{Value: navToLower(s.Value)}, nil
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
