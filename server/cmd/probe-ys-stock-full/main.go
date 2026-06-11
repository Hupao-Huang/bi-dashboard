// probe-ys-stock-full 一次性探针: 验证"按组织全量查现存量"(空货品编码)的可行性
// — 行数多大 / 耗时多久 / 接口会不会截断。给出库工具拆单批量化(3次调用替代N×3次)做依据。
package main

import (
	"fmt"
	"log"
	"time"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/yonsuite"
)

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("load cfg: %v", err)
	}
	c := yonsuite.NewClient(cfg.YonSuite.AppKey, cfg.YonSuite.AppSecret, cfg.YonSuite.BaseURL)

	orgs := []struct{ id, name string }{
		{"2451285875823214599", "浙江松鲜鲜"},
		{"2451285927362822152", "杭州润松"},
		{"2451285918772887559", "杭州华鲜"},
	}
	for _, o := range orgs {
		t0 := time.Now()
		rows, err := c.QueryStockByCondition(o.id, "", "", "", "")
		el := time.Since(t0)
		if err != nil {
			fmt.Printf("%s: ❌ %v (耗时 %.1fs)\n", o.name, err, el.Seconds())
			continue
		}
		// 统计去重货品数, 看覆盖面
		prods := map[string]bool{}
		for _, r := range rows {
			prods[r.ProductCode] = true
		}
		fmt.Printf("%s: %d 行 / %d 个货品, 耗时 %.1fs\n", o.name, len(rows), len(prods), el.Seconds())

		// 一致性抽查: 挑全量结果里行数最多的货品, 按编码单查一次, 比对行数和可用量合计
		if o.id == "2451285875823214599" && len(rows) > 0 {
			cnt := map[string]int{}
			for _, r := range rows {
				cnt[r.ProductCode]++
			}
			pick, best := "", 0
			for p, n := range cnt {
				if n > best {
					pick, best = p, n
				}
			}
			fullN, fullQty := 0, 0.0
			for _, r := range rows {
				if r.ProductCode == pick {
					fullN++
					fullQty += r.AvailableQty
				}
			}
			single, err := c.QueryStockByCondition(o.id, pick, "", "", "")
			if err != nil {
				fmt.Printf("  抽查 %s: 单查失败 %v\n", pick, err)
				continue
			}
			singleQty := 0.0
			for _, r := range single {
				singleQty += r.AvailableQty
			}
			match := "✅ 一致"
			if fullN != len(single) || fullQty != singleQty {
				match = "❌ 不一致!!"
			}
			fmt.Printf("  抽查 %s: 全量里 %d 行/可用 %.0f vs 单查 %d 行/可用 %.0f → %s\n",
				pick, fullN, fullQty, len(single), singleQty, match)
		}
	}
}
