package handler

// 合思发票货物明细行名称拉取 (规则 18 用)
// 接口: POST /api/openapi/v2/extension/INVOICE/object/invoice/detailBatch {"invoiceIds":[...]}
// 返回 E_system_发票明细_name = 发票票面"项目名称"列 (例 "*印刷品*KT板立牌")。
// 本地 hesi_flow_invoice 没存货物行, 现场拉 + 5min 缓存 (待办列表才触发, 广告费明细很少)

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

var (
	hesiInvItemCache   = map[string][]string{}
	hesiInvItemCacheAt = map[string]time.Time{}
	hesiInvItemMu      sync.Mutex
)

// fetchInvoiceItemNames 批量拉发票货物明细行名称, 返回 invoiceID → 项目名称列表
func (h *DashboardHandler) fetchInvoiceItemNames(invoiceIDs []string) (map[string][]string, error) {
	hesiInvItemMu.Lock()
	defer hesiInvItemMu.Unlock()
	out := map[string][]string{}
	var missing []string
	now := time.Now()
	for _, id := range invoiceIDs {
		if at, ok := hesiInvItemCacheAt[id]; ok && now.Sub(at) < hesiDictTTL {
			out[id] = hesiInvItemCache[id]
		} else {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	token, err := h.getHesiToken()
	if err != nil {
		return out, err
	}
	// 接口批量上限 100 (同发票主体接口口径, 别学附件接口 200 翻车)
	for i := 0; i < len(missing); i += 100 {
		end := i + 100
		if end > len(missing) {
			end = len(missing)
		}
		body, _ := json.Marshal(map[string]interface{}{"invoiceIds": missing[i:end]})
		resp, err := hesiHTTP.Post(
			fmt.Sprintf("%s/api/openapi/v2/extension/INVOICE/object/invoice/detailBatch?accessToken=%s", hesiAPIBase, token),
			"application/json", bytes.NewReader(body))
		if err != nil {
			return out, err
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			snip := string(data)
			if len(snip) > 200 {
				snip = snip[:200]
			}
			return out, fmt.Errorf("合思返回 HTTP %d: %s", resp.StatusCode, snip)
		}
		var parsed struct {
			Items []struct {
				MasterID string `json:"masterId"`
				Name     string `json:"E_system_发票明细_name"`
			} `json:"items"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			snip := string(data)
			if len(snip) > 200 {
				snip = snip[:200]
			}
			return out, fmt.Errorf("解析发票明细失败: %w (body: %s)", err, snip)
		}
		got := map[string][]string{}
		for _, it := range parsed.Items {
			if it.MasterID != "" && it.Name != "" {
				got[it.MasterID] = append(got[it.MasterID], it.Name)
			}
		}
		for _, id := range missing[i:end] {
			hesiInvItemCache[id] = got[id] // 没明细的也缓存空值, 防止反复打接口
			hesiInvItemCacheAt[id] = time.Now()
			out[id] = got[id]
		}
	}
	return out, nil
}
