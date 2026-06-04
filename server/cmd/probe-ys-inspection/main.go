// probe-ys-inspection 探针: 拉来料检验单, 确认账号权限/数据量/不合格字段
// 用法: ./probe-ys-inspection            # 不带日期过滤, 拉第一页看总量
//       ./probe-ys-inspection 2026-01-01 2026-06-04  # 按检验日期范围
package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/yonsuite"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

func gs(rec map[string]interface{}, key string) string {
	v, ok := rec[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return x.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func main() {
	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if cfg.YonSuite.AppKey == "" || cfg.YonSuite.AppSecret == "" || cfg.YonSuite.BaseURL == "" {
		log.Fatalf("config.json 缺少 yonsuite 配置")
	}
	client := yonsuite.NewClient(cfg.YonSuite.AppKey, cfg.YonSuite.AppSecret, cfg.YonSuite.BaseURL)

	req := &yonsuite.InspectionListReq{
		Billnum:   "qms_incominspectorder_list", // 来料检验单
		PageIndex: 1,
		PageSize:  500,
	}
	if len(os.Args) >= 3 {
		req.Simple = &yonsuite.InspectionSimple{
			OpenInspectDateBegin: os.Args[1],
			OpenInspectDateEnd:   os.Args[2],
		}
		fmt.Printf("按检验日期过滤: %s ~ %s\n", os.Args[1], os.Args[2])
	} else {
		fmt.Printf("不带日期过滤, 拉第一页看总量\n")
	}

	resp, err := client.QueryInspectionList(req)
	if err != nil {
		log.Fatalf("调接口失败: %v", err)
	}

	fmt.Printf("\n========== 来料检验单 ==========\n")
	fmt.Printf("总数 recordCount=%d, 本页返回 %d 条, pageCount=%d\n\n",
		resp.Data.RecordCount, len(resp.Data.RecordList), resp.Data.PageCount)

	if len(resp.Data.RecordList) == 0 {
		fmt.Println("⚠️  没拉到数据 (账号无权限 / 无来料检验单 / 过滤太窄)")
		return
	}

	// 统计不合格 (合格等级=不合格 / 库存状态名=不合格 / 处理方式=拒收)
	bad := 0
	for _, rec := range resp.Data.RecordList {
		grade := gs(rec, "pk_qualify_grade")
		stockStatus := gs(rec, "pk_stockstatus_statusName")
		handle := gs(rec, "handleType_name")
		if strings.Contains(grade, "不合格") || strings.Contains(stockStatus, "不合格") || strings.Contains(handle, "拒收") || strings.Contains(handle, "退货") {
			bad++
		}
	}
	fmt.Printf("本页 %d 条里, 判定为「不合格/拒收/退货」的有 %d 条\n\n", len(resp.Data.RecordList), bad)

	// 打印前 15 条关键字段
	show := len(resp.Data.RecordList)
	if show > 15 {
		show = 15
	}
	fmt.Printf("前 %d 条明细:\n", show)
	for i := 0; i < show; i++ {
		rec := resp.Data.RecordList[i]
		fmt.Printf("[%2d] 单号=%s | 类型=%s | 检验日=%s | 物料=%s(%s) | 批次=%s | 供应商=%s | 数量=%s\n",
			i+1, gs(rec, "code"), gs(rec, "trantype_name"), gs(rec, "inspectDate"),
			gs(rec, "pk_material_name"), gs(rec, "pk_material_code"), gs(rec, "pk_batchcode"),
			gs(rec, "pk_outsupplier_name"), gs(rec, "qty"))
		fmt.Printf("       状态verifystate=%s | 检验结果inspectResult=%s | 合格等级=%s | 库存状态=%s | 处理方式=%s | 合格数qnum=%s 不合格数nqnum=%s 合格率qrate=%s\n",
			gs(rec, "verifystate"), gs(rec, "inspectResult"), gs(rec, "pk_qualify_grade"),
			gs(rec, "pk_stockstatus_statusName"), gs(rec, "handleType_name"),
			gs(rec, "qnum"), gs(rec, "nqnum"), gs(rec, "qrate"))
	}

	// 打印第 1 条的全部字段名 (定表结构用)
	fmt.Printf("\n第 1 条的全部字段名 (共 %d 个, 定表用):\n", len(resp.Data.RecordList[0]))
	keys := make([]string, 0, len(resp.Data.RecordList[0]))
	for k := range resp.Data.RecordList[0] {
		keys = append(keys, k)
	}
	fmt.Println(strings.Join(keys, ", "))
}
