package handler

import "testing"

func TestRatioPerOrderPallets(t *testing.T) {
	if ratio(30, 120) != 0.25 {
		t.Errorf("ratio 25%% 错")
	}
	if ratio(1, 0) != 0 {
		t.Errorf("ratio total=0 应 0")
	}
	if perOrder(300, 100) != 3 {
		t.Errorf("perOrder 错")
	}
	if perOrder(5, 0) != 0 {
		t.Errorf("perOrder orders=0 应 0")
	}
	if palletsOf(128, 64) != 2 {
		t.Errorf("palletsOf 错")
	}
	if palletsOf(10, 0) != 0 {
		t.Errorf("palletsOf 无托规应 0")
	}
}

func TestRollupPlatforms(t *testing.T) {
	in := []ChannelRow{
		{Platform: "电商", Channel: "天猫", Orders: 10, Bottles: 100, WeightKg: 20},
		{Platform: "社媒", Channel: "抖音", Orders: 30, Bottles: 300, WeightKg: 60},
		{Platform: "电商", Channel: "京东", Orders: 5, Bottles: 50, WeightKg: 10},
	}
	out := rollupPlatforms(in)
	// 期望顺序: 社媒合计 → 抖音 → 电商合计 → 天猫 → 京东 → 总计
	if out[0].Channel != "社媒合计" || out[0].Orders != 30 {
		t.Fatalf("社媒合计不对: %+v", out[0])
	}
	if out[1].Channel != "抖音" {
		t.Fatalf("社媒明细应紧跟合计: %+v", out[1])
	}
	if out[2].Channel != "电商合计" || out[2].Orders != 15 || out[2].Bottles != 150 {
		t.Fatalf("电商合计不对: %+v", out[2])
	}
	// 电商块内按 Bottles 降序: 天猫(100) 在 京东(50) 前
	if out[3].Channel != "天猫" || out[4].Channel != "京东" {
		t.Fatalf("电商块内应按销量降序: %+v %+v", out[3], out[4])
	}
	last := out[len(out)-1]
	if last.Channel != "总计" || last.Orders != 45 || last.Bottles != 450 || last.WeightKg != 90 {
		t.Fatalf("总计不对: %+v", last)
	}
}
