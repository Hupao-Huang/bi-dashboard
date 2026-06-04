// probe-ys-inspection-detail 探针: 拉单张检验单详情, dump 结构找不合格原因
// 用法: ./probe-ys-inspection-detail [检验单号]   默认 LLJY202606040018(报废单)
package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/yonsuite"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

func main() {
	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("配置: %v", err)
	}
	client := yonsuite.NewClient(cfg.YonSuite.AppKey, cfg.YonSuite.AppSecret, cfg.YonSuite.BaseURL)

	code := "LLJY202606040018"
	if len(os.Args) >= 2 {
		code = os.Args[1]
	}
	fmt.Printf("拉检验单详情: code=%s\n\n", code)

	data, err := client.QueryInspectionDetail(map[string]string{"code": code})
	if err != nil {
		log.Fatalf("调详情失败: %v", err)
	}

	// 1) 顶层字段: 标量打印, 数组/对象只报类型+数量
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println("===== 顶层字段 =====")
	childArrays := map[string][]interface{}{}
	for _, k := range keys {
		v := data[k]
		switch x := v.(type) {
		case []interface{}:
			fmt.Printf("  %s = [数组 %d 项]\n", k, len(x))
			childArrays[k] = x
		case map[string]interface{}:
			fmt.Printf("  %s = {对象 %d 字段}\n", k, len(x))
		default:
			s := fmt.Sprintf("%v", v)
			if strings.TrimSpace(s) != "" && s != "<nil>" {
				fmt.Printf("  %s = %s\n", k, s)
			}
		}
	}

	// 2) 子数组(检验项/结果明细): 打印第一项的全字段, 重点找原因
	for name, arr := range childArrays {
		if len(arr) == 0 {
			continue
		}
		fmt.Printf("\n===== 子数组 %s (共 %d 项), 第1项全字段 =====\n", name, len(arr))
		if first, ok := arr[0].(map[string]interface{}); ok {
			fk := make([]string, 0, len(first))
			for k := range first {
				fk = append(fk, k)
			}
			sort.Strings(fk)
			for _, k := range fk {
				s := fmt.Sprintf("%v", first[k])
				if strings.TrimSpace(s) != "" && s != "<nil>" && s != "map[]" && s != "[]" {
					fmt.Printf("    %s = %s\n", k, s)
				}
			}
		}
	}

	// 3) 全文搜原因相关关键词
	fmt.Printf("\n===== 全文搜「原因/不良/缺陷/不合格/备注/结论」 =====\n")
	raw, _ := json.Marshal(data)
	for _, kw := range []string{"原因", "不良", "缺陷", "不合格", "结论", "判定", "描述"} {
		if strings.Contains(string(raw), kw) {
			fmt.Printf("  ✓ 出现关键词「%s」\n", kw)
		}
	}
}
