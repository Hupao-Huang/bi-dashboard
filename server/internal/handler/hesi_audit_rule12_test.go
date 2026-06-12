package handler

// 规则 12-1 私车公用按行车记录自动对账 (跑哥 2026-06-12 升级, 不再一律人工核)

import (
	"strings"
	"testing"
	"time"
)

// seedDriveRecCache 注入行车记录缓存 (绕开合思 API)
func seedDriveRecCache(recs map[string]*DriveRecord) func() {
	hesiDriveRecMu.Lock()
	old, oldAt := hesiDriveRecCache, hesiDriveRecCacheAt
	hesiDriveRecCache = recs
	hesiDriveRecCacheAt = time.Now()
	hesiDriveRecMu.Unlock()
	return func() {
		hesiDriveRecMu.Lock()
		hesiDriveRecCache, hesiDriveRecCacheAt = old, oldAt
		hesiDriveRecMu.Unlock()
	}
}

func mkDriveRaw(amt, recID string) map[string]interface{} {
	form := map[string]interface{}{
		"detailId": "D-drive", "detailNo": float64(3),
		"amount": map[string]interface{}{"standard": amt},
	}
	if recID != "" {
		form["u_行车记录"] = recID
	}
	return map[string]interface{}{
		"details": []interface{}{
			map[string]interface{}{"feeTypeId": driveFeeTypeID, "feeTypeForm": form},
		},
	}
}

func TestRule121AmountMatchesSubsidyPasses(t *testing.T) {
	// 报销 68.38 = 系统算出 68.38 → 自动通过, 无人工核
	restore := seedDriveRecCache(map[string]*DriveRecord{
		"REC1": {Mileage: "97.68", Standard: "0.70", Subsidy: "68.38"},
	})
	defer restore()
	h := &DashboardHandler{}
	rej, warn := h.ruleDriveRecordCheck(mkDriveRaw("68.38", "REC1"))
	if len(rej) != 0 || len(warn) != 0 {
		t.Errorf("金额=系统补助应自动通过: rej=%v warn=%v", rej, warn)
	}
}

func TestRule121OverSubsidyRejects(t *testing.T) {
	// 报销 80 > 系统算出 68.38 → 建议驳回
	restore := seedDriveRecCache(map[string]*DriveRecord{
		"REC1": {Mileage: "97.68", Standard: "0.70", Subsidy: "68.38"},
	})
	defer restore()
	h := &DashboardHandler{}
	rej, _ := h.ruleDriveRecordCheck(mkDriveRaw("80.00", "REC1"))
	got := strings.Join(rej, "; ")
	if !strings.Contains(got, "规则 12-1") || !strings.Contains(got, "97.68") {
		t.Errorf("超出系统补助应驳回并带里程, got %q", got)
	}
}

func TestRule121UnderSubsidyPasses(t *testing.T) {
	// 报销 50 < 系统算出 68.38 → 少报不拦
	restore := seedDriveRecCache(map[string]*DriveRecord{
		"REC1": {Mileage: "97.68", Standard: "0.70", Subsidy: "68.38"},
	})
	defer restore()
	h := &DashboardHandler{}
	rej, warn := h.ruleDriveRecordCheck(mkDriveRaw("50.00", "REC1"))
	if len(rej) != 0 || len(warn) != 0 {
		t.Errorf("少报不应拦: rej=%v warn=%v", rej, warn)
	}
}

func TestRule121NoRecordFallsBackManual(t *testing.T) {
	// 没挂行车记录 → 保留人工核提示
	restore := seedDriveRecCache(map[string]*DriveRecord{})
	defer restore()
	h := &DashboardHandler{}
	rej, warn := h.ruleDriveRecordCheck(mkDriveRaw("68.38", ""))
	if len(rej) != 0 {
		t.Errorf("无行车记录不应驳回, got %v", rej)
	}
	if !strings.Contains(strings.Join(warn, "; "), "人工核") {
		t.Errorf("无行车记录应转人工核, got %v", warn)
	}
}
