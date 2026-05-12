package handler

// 同步工具实时日志查看 — sync-daily-trades 等独立工具自己 log.SetOutput 到固定文件,
// bi-server 给的 cmd.Stdout 拿不到内容. 这里直接读固定 log 末尾 N 行返回.
// v1.58.1: fallback 找最新 manual-{key}-*.log (bi-server 手动触发 cmd.Stdout 写的)
// 应对场景: 工具用 fmt.Printf 而非 log.Printf 时, 固定 log 几乎空, 真实进度在 manual-*.log

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

// findLatestManualLog 找 logBaseDir 下 manual-{key}-*.log 最新一个 (按 mtime)
func findLatestManualLog(key string) string {
	pattern := `C:\Users\Administrator\bi-dashboard\server\manual-` + key + `-*.log`
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}
	type fInfo struct {
		path string
		info os.FileInfo
	}
	var list []fInfo
	for _, p := range matches {
		if i, err := os.Stat(p); err == nil {
			list = append(list, fInfo{p, i})
		}
	}
	if len(list) == 0 {
		return ""
	}
	sort.Slice(list, func(i, j int) bool { return list[i].info.ModTime().After(list[j].info.ModTime()) })
	return list[0].path
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
	usedPath := path

	// v1.58.1: 固定 log 太空时 fallback 找最新 manual-{key}-*.log
	// 兼容工具用 fmt.Printf 输出 (走 stdout, 不进固定 log) 的情况
	if len(tail) < 5 {
		if manual := findLatestManualLog(key); manual != "" {
			manualTail := readLastNLines(manual, lines)
			if len(manualTail) > len(tail) {
				tail = manualTail
				usedPath = manual
			}
		}
	}

	writeJSON(w, map[string]interface{}{
		"key":   key,
		"path":  usedPath,
		"lines": tail,
		"text":  strings.Join(tail, "\n"),
		"count": len(tail),
	})
}
