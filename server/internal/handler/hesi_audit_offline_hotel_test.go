package handler

// 线下(世创/世用)住宿标准 规则 7-3 线下分支 (樊雪娇 2026-06-17)
// 线下用专属住宿表(3档职级×3档城市), 新一线→二线; 职级映射同线下补贴口径延伸
// 集团口径放过的, 线下口径(更严)要拦住

import (
	"strings"
	"testing"
)

// 职级映射: 集团总监/副总裁/总裁→集团总监; 集团经理(大区经理归此档)→大区经理及以上; 其余→其他员工
func TestOfflineHotelLevel(t *testing.T) {
	cases := map[string]string{
		"集团总监": "集团总监", "副总裁": "集团总监", "总裁": "集团总监",
		"集团经理":  "大区经理及以上",
		"主管和其他": "其他员工", "": "其他员工", "大区经理": "其他员工", // 大区经理非花名册档, 实际花名册里没有, 兜底其他员工
	}
	for in, want := range cases {
		if got := offlineHotelLevel(in); got != want {
			t.Errorf("offlineHotelLevel(%q)=%q want %q", in, got, want)
		}
	}
}

// 城市映射: 一线→一线, 新一线→二线, 二线→二线, 其他→国内其他
func TestOfflineCityTier(t *testing.T) {
	cases := map[string]string{
		"一线": "一线", "新一线": "二线", "二线": "二线", "其他": "国内其他",
	}
	for in, want := range cases {
		if got := offlineCityTier(in); got != want {
			t.Errorf("offlineCityTier(%q)=%q want %q", in, got, want)
		}
	}
}

// 带城市的住宿明细 (mkHotelRaw 不含 city, 线下测试要城市才能定档)
func mkHotelRawWithCity(amt, city string, startMs, endMs int64) map[string]interface{} {
	form := map[string]interface{}{
		"detailNo": float64(1),
		"amount":   map[string]interface{}{"standard": amt},
		"city":     city,
	}
	if startMs != 0 || endMs != 0 {
		form["feeDatePeriod"] = map[string]interface{}{"start": float64(startMs), "end": float64(endMs)}
	}
	return map[string]interface{}{"details": []interface{}{
		map[string]interface{}{"feeTypeId": hotelFeeTypeID, "feeTypeForm": form},
	}}
}

// 同一笔: 集团口径过(主管和其他一线400), 线下口径超标(其他员工一线350)
func TestRule73OfflineStricterThanGroup(t *testing.T) {
	restore := seedCityTierCache(map[string]string{"北京": "一线"})
	defer restore()
	h := &DashboardHandler{} // DB nil → 标准走代码默认值; submitterID 空 → fallback (集团 主管和其他 / 线下 其他员工)
	raw := mkHotelRawWithCity("360", "北京", r15June1, r15June1)

	// 集团口径: 360 < 400 不触发
	rejG, warnG := h.ruleAccommodationStandard(raw, "", false)
	if rejG != "" || warnG != "" {
		t.Errorf("集团口径 360<400(一线主管和其他) 不应触发, rej=%q warn=%q", rejG, warnG)
	}
	// 线下口径: 360 > 350 超标 (fallback → warn)
	rejO, warnO := h.ruleAccommodationStandard(raw, "", true)
	combined := rejO + warnO
	if !strings.Contains(combined, "其他员工") || !strings.Contains(combined, "线下") {
		t.Errorf("线下口径 360>350(一线其他员工) 应判超标且标线下其他员工, rej=%q warn=%q", rejO, warnO)
	}
}

// 新一线城市 → 线下二线档 (其他员工二线 280)
func TestRule73OfflineNewFirstTierMapsToSecond(t *testing.T) {
	restore := seedCityTierCache(map[string]string{"杭州": "新一线"})
	defer restore()
	h := &DashboardHandler{}
	raw := mkHotelRawWithCity("300", "杭州", r15June1, r15June1)
	rejO, warnO := h.ruleAccommodationStandard(raw, "", true)
	combined := rejO + warnO
	if !strings.Contains(combined, "二线") {
		t.Errorf("新一线应映射到线下二线档(其他员工280), 300>280 应超标, rej=%q warn=%q", rejO, warnO)
	}
}

// 线下口径下符合标准的不触发 (其他员工一线 350, 报 ¥340)
func TestRule73OfflineWithinStandardPasses(t *testing.T) {
	restore := seedCityTierCache(map[string]string{"北京": "一线"})
	defer restore()
	h := &DashboardHandler{}
	raw := mkHotelRawWithCity("340", "北京", r15June1, r15June1)
	rejO, warnO := h.ruleAccommodationStandard(raw, "", true)
	if rejO != "" || warnO != "" {
		t.Errorf("线下 340<350(一线其他员工) 应通过, rej=%q warn=%q", rejO, warnO)
	}
}
