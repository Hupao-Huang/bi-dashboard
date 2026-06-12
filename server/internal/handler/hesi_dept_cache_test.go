package handler

// 2026-06-12 第二批: 部门树 N+1 改进程缓存后的语义回归
// 原版逐层 SELECT vs 新版内存链走查, 判定结果必须一致

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func seedhesiDeptTreeCache(m map[string]hesiDeptNode) func() {
	hesiDeptTreeCacheMu.Lock()
	old, oldAt := hesiDeptTreeCache, hesiDeptTreeCacheAt
	hesiDeptTreeCache = m
	hesiDeptTreeCacheAt = time.Now()
	hesiDeptTreeCacheMu.Unlock()
	return func() {
		hesiDeptTreeCacheMu.Lock()
		hesiDeptTreeCache, hesiDeptTreeCacheAt = old, oldAt
		hesiDeptTreeCacheMu.Unlock()
	}
}

func TestRuleDeptLeafSemantics(t *testing.T) {
	restore := seedhesiDeptTreeCache(map[string]hesiDeptNode{
		"L1": {name: "销售一组", parentID: "P1", hasChild: false, active: true},
		"P1": {name: "销售部", parentID: "", hasChild: true, active: true},
		"X1": {name: "已停用组", parentID: "P1", hasChild: false, active: false},
	})
	defer restore()
	h := &DashboardHandler{}

	if got := h.ruleDeptLeaf("", "发起人部门 (规则 1)"); got != "发起人部门 (规则 1) 为空" {
		t.Errorf("空部门应报为空, got %q", got)
	}
	if got := h.ruleDeptLeaf("L1", "发起人部门 (规则 1)"); got != "" {
		t.Errorf("末级部门应通过, got %q", got)
	}
	if got := h.ruleDeptLeaf("P1", "发起人部门 (规则 1)"); got != "发起人部门 (规则 1)「销售部」非末级" {
		t.Errorf("非末级应报非末级, got %q", got)
	}
	// 原 SQL 带 active=1: 停用部门 = 查无 = 跳过规则
	if got := h.ruleDeptLeaf("X1", "发起人部门 (规则 1)"); got != "" {
		t.Errorf("停用部门应跳过规则, got %q", got)
	}
	// 不在表的新部门 = 跳过
	if got := h.ruleDeptLeaf("NOPE", "发起人部门 (规则 1)"); got != "" {
		t.Errorf("未同步部门应跳过规则, got %q", got)
	}
}

func TestDeptChainContains(t *testing.T) {
	restore := seedhesiDeptTreeCache(map[string]hesiDeptNode{
		"D3":   {name: "口味组", parentID: "D2", active: true},
		"D2":   {name: "产品研发中心", parentID: "D1", active: true},
		"D1":   {name: "集团", parentID: "", active: true},
		"SELF": {name: "自环部门", parentID: "SELF", active: true}, // parent 指向自己, 原版 break 防死循环
	})
	defer restore()
	h := &DashboardHandler{}

	if !h.isResearchDept("D3") {
		t.Error("祖先链含'研发'应判 true (D3→D2 产品研发中心)")
	}
	if h.isResearchDept("D1") {
		t.Error("链上无'研发'应判 false")
	}
	if h.isResearchDept("") {
		t.Error("空部门应判 false")
	}
	if h.isResearchDept("SELF") {
		t.Error("自环部门应 break 返回 false, 不能死循环")
	}
	if h.isBrandCenterDept("D3") {
		t.Error("链上无'品牌中心'应判 false")
	}
}

func TestCacheRecorderUnwrapAndFlush(t *testing.T) {
	// statusRecorder 同款病第二例的回归锁: 缓存包装必须能被 ResponseController 穿透
	rec := httptest.NewRecorder()
	w := &cacheResponseRecorder{ResponseWriter: rec, statusCode: 200}
	if w.Unwrap() != http.ResponseWriter(rec) {
		t.Error("Unwrap 应返回底层 ResponseWriter")
	}
	w.Flush() // httptest.ResponseRecorder 实现 Flusher, 不应 panic
	if !rec.Flushed {
		t.Error("Flush 应透传到底层 Flusher")
	}
}
