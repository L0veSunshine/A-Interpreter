package object

import (
	"fmt"
	"strconv"
	"strings"
)

type MapPair struct {
	Key,
	Item Object
}

func (p *MapPair) Inspect() string {
	var key, item string
	if p.Key.Type() == StringObj {
		key = strconv.Quote(p.Key.Inspect())
	} else {
		key = p.Key.Inspect()
	}
	if p.Item.Type() == StringObj {
		item = strconv.Quote(p.Item.Inspect())
	} else {
		item = p.Item.Inspect()
	}
	return key + ": " + item
}

type Map struct {
	Store map[int]MapPair
	Size  int
}

func (m Map) Type() ObjType {
	return MapObj
}

func (m Map) Inspect() string {
	var sb strings.Builder
	sb.WriteString("{")
	if m.Size > 0 {
		idx := 1
		for _, p := range m.Store {
			if idx != m.Size {
				sb.WriteString(p.Inspect() + ", ")
			} else {
				sb.WriteString(p.Inspect())
			}
			idx += 1
		}
	}
	fmt.Println(len(m.Store))
	sb.WriteString("}")
	return sb.String()
}
