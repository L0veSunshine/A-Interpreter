package object

import (
	"sort"
	"strconv"
	"strings"
)

type MapPair struct {
	Key,
	Item Object
}

func (p MapPair) Inspect() string {
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
	if len(m.Store) > 0 {
		// Go map 的遍历顺序是随机的，排序后输出才能保证结果可复现
		hashes := make([]int, 0, len(m.Store))
		for h := range m.Store {
			hashes = append(hashes, h)
		}
		sort.Ints(hashes)
		for i, h := range hashes {
			sb.WriteString(m.Store[h].Inspect())
			if i != len(hashes)-1 {
				sb.WriteString(", ")
			}
		}
	}
	sb.WriteString("}")
	return sb.String()
}
