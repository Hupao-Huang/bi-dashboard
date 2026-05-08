package importutil

import "strings"

// HeaderIdx 把表头行转成 名称→列索引 的 map. 重复名称保留首次.
func HeaderIdx(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if _, ok := m[h]; !ok {
			m[h] = i
		}
	}
	return m
}

// FindCol 从 HeaderIdx 返回的 map 里, 按多个候选名找列索引. 找不到返 -1.
func FindCol(idx map[string]int, aliases ...string) int {
	for _, a := range aliases {
		if i, ok := idx[a]; ok {
			return i
		}
	}
	return -1
}
