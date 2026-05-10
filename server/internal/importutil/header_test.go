package importutil

import "testing"

func TestHeaderIdx(t *testing.T) {
	header := []string{"商品名称", "  销售额  ", "数量", "", "渠道", "数量"}
	m := HeaderIdx(header)

	// 正常 trim
	if i, ok := m["商品名称"]; !ok || i != 0 {
		t.Errorf("商品名称: idx=%d ok=%v", i, ok)
	}
	if i, ok := m["销售额"]; !ok || i != 1 {
		t.Errorf("销售额 (trim) idx=%d", i)
	}
	if i, ok := m["渠道"]; !ok || i != 4 {
		t.Errorf("渠道 idx=%d", i)
	}
	// 重复保留首次出现
	if i := m["数量"]; i != 2 {
		t.Errorf("数量 重复时应保留首次出现 idx=2, got %d", i)
	}
	// 空字符串不进 map
	if _, ok := m[""]; ok {
		t.Error("空 header 不应进 map")
	}
}

func TestFindCol(t *testing.T) {
	idx := map[string]int{"商品名称": 0, "销售额": 1, "数量": 2}

	// 单候选命中
	if i := FindCol(idx, "商品名称"); i != 0 {
		t.Errorf("商品名称 命中 idx=0, got %d", i)
	}

	// 多候选取首个命中
	if i := FindCol(idx, "商品标题", "商品名称", "Title"); i != 0 {
		t.Errorf("多候选首命中应返回 0, got %d", i)
	}

	// 全部不命中返 -1
	if i := FindCol(idx, "Sales", "Quantity"); i != -1 {
		t.Errorf("全不命中应返 -1, got %d", i)
	}

	// 无 alias 返 -1
	if i := FindCol(idx); i != -1 {
		t.Errorf("无 alias 应返 -1, got %d", i)
	}
}
