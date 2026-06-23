package handler

import "testing"

// cfGroups 是与 cfMetrics 平行维护的分组清单, 两者容易漂移。此测试在 CI 卡住两类漂移:
//   正向: 新增指标到 cfMetrics 却忘了归组 → 自定义指标弹窗里落到"其他指标"
//   反向: cfMetrics 改名/删除却在 cfGroups 留下死条目 → 该 key 永不命中, 静默腐烂
func TestCfGroupsCoverAllMetrics(t *testing.T) {
	inGroup := map[string]int{}
	for _, g := range cfGroups {
		for _, k := range g.Keys {
			inGroup[k]++
		}
	}
	for _, m := range cfMetrics {
		switch inGroup[m.Key] {
		case 0:
			t.Errorf("指标 %s (%s) 未归入任何 cfGroups 分组, 会落到'其他指标'", m.Key, m.Label)
		case 1:
			// ok
		default:
			t.Errorf("指标 %s 在 cfGroups 出现 %d 次, 应恰好 1 次", m.Key, inGroup[m.Key])
		}
	}

	metricSet := map[string]bool{}
	for _, m := range cfMetrics {
		metricSet[m.Key] = true
	}
	for _, g := range cfGroups {
		for _, k := range g.Keys {
			if !metricSet[k] {
				t.Errorf("cfGroups 分组 %q 含 cfMetrics 不存在的 key %q (已改名/删除?)", g.Name, k)
			}
		}
	}
}
