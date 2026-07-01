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

// cr 造一个带当日/当月两组的渠道行(简化测试书写)
func cr(platform, channel string, tOrders int, tBottles, tWeight float64, mOrders int, mBottles, mWeight float64) ChannelRow {
	return ChannelRow{
		Platform: platform, Channel: channel,
		Today: ChannelStat{Orders: tOrders, Bottles: tBottles, WeightKg: tWeight},
		Month: ChannelStat{Orders: mOrders, Bottles: mBottles, WeightKg: mWeight},
	}
}

// 同渠道名跨两个平台(编辑器允许手动配)时, rollup 必须各自独立不串
func TestRollupPlatforms_SameChannelCrossPlatform(t *testing.T) {
	in := []ChannelRow{
		cr("电商", "分销", 2, 20, 4, 10, 100, 20),
		cr("社媒", "分销", 1, 10, 2, 5, 50, 10),
	}
	out := rollupPlatforms(in)
	last := out[len(out)-1]
	if last.Channel != "总计" || last.Month.Orders != 15 || last.Month.Bottles != 150 || last.Today.Orders != 3 {
		t.Fatalf("同渠道跨平台总计应 当月15/150 当日3, got %+v", last)
	}
	cnt := 0
	for _, r := range out {
		if r.Channel == "分销" {
			cnt++
		}
	}
	if cnt != 2 {
		t.Fatalf("两个平台下的『分销』明细行都应保留, got %d", cnt)
	}
}

func TestRollupPlatforms(t *testing.T) {
	in := []ChannelRow{
		cr("电商", "天猫", 1, 10, 2, 10, 100, 20),
		cr("社媒", "抖音", 3, 30, 6, 30, 300, 60),
		cr("电商", "京东", 0, 0, 0, 5, 50, 10),
	}
	out := rollupPlatforms(in)
	// 期望顺序: 社媒合计 → 抖音 → 电商合计 → 天猫 → 京东 → 总计(按当月 Bottles 排)
	if out[0].Channel != "社媒合计" || out[0].Month.Orders != 30 {
		t.Fatalf("社媒合计不对: %+v", out[0])
	}
	if out[1].Channel != "抖音" {
		t.Fatalf("社媒明细应紧跟合计: %+v", out[1])
	}
	if out[2].Channel != "电商合计" || out[2].Month.Orders != 15 || out[2].Month.Bottles != 150 {
		t.Fatalf("电商合计不对: %+v", out[2])
	}
	// 电商块内按当月 Bottles 降序: 天猫(100) 在 京东(50) 前
	if out[3].Channel != "天猫" || out[4].Channel != "京东" {
		t.Fatalf("电商块内应按当月销量降序: %+v %+v", out[3], out[4])
	}
	last := out[len(out)-1]
	if last.Channel != "总计" || last.Month.Orders != 45 || last.Month.Bottles != 450 || last.Month.WeightKg != 90 {
		t.Fatalf("当月总计不对: %+v", last)
	}
	// 当日也要正确累加(天猫1+抖音3+京东0=4)
	if last.Today.Orders != 4 || last.Today.Bottles != 40 {
		t.Fatalf("当日总计不对: %+v", last.Today)
	}
}
