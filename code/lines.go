package code

// LineEntry 记录一段指令起始偏移对应的源码行号。
// 编译器在源码行发生变化时追加一条，VM 在报错时用它反查行号。
type LineEntry struct {
	Offset int
	Line   int
}

// LineAt 返回指令偏移 ip 所在的源码行；没有映射时返回 0。
// entries 按 Offset 升序排列，用二分查找。
func LineAt(entries []LineEntry, ip int) int {
	if len(entries) == 0 {
		return 0
	}
	lo, hi := 0, len(entries)-1
	best := -1
	for lo <= hi {
		mid := (lo + hi) / 2
		if entries[mid].Offset <= ip {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if best < 0 {
		return 0
	}
	return entries[best].Line
}
