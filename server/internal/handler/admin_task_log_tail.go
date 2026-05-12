package handler

// 同步工具实时日志查看 — sync-daily-trades 等独立工具自己 log.SetOutput 到固定文件,
// bi-server 给的 cmd.Stdout 拿不到内容. 这里直接读固定 log 末尾 N 行返回.

import (
	"net/http"
	"strconv"
	"strings"
)

// fixedToolLogMap task key → 工具自己写死的 log 文件绝对路径
var fixedToolLogMap = map[string]string{
	"sync-trades":    `C:\Users\Administrator\bi-dashboard\server\sync-daily-trades.log`,
	"sync-summary":   `C:\Users\Administrator\bi-dashboard\server\sync-daily-summary.log`,
	"snapshot-stock": `C:\Users\Administrator\bi-dashboard\server\snapshot-stock.log`,
	"sync-hesi":      `C:\Users\Administrator\bi-dashboard\server\sync-hesi.log`,
}

// AdminSyncToolLog GET /api/admin/sync-tools/log?key=sync-trades&lines=200
// 返回工具固定 log 末尾 N 行 (不依赖 runningTasks 内存)
func (h *DashboardHandler) AdminSyncToolLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, "缺少 key 参数")
		return
	}
	path, ok := fixedToolLogMap[key]
	if !ok {
		writeError(w, http.StatusBadRequest, "未知 key: "+key)
		return
	}

	lines := 200
	if s := r.URL.Query().Get("lines"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 2000 {
			lines = n
		}
	}

	tail := readLastNLines(path, lines)
	writeJSON(w, map[string]interface{}{
		"key":   key,
		"path":  path,
		"lines": tail,
		"text":  strings.Join(tail, "\n"),
		"count": len(tail),
	})
}
