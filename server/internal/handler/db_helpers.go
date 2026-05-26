package handler

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"
)

// defaultQueryTimeout v1.74.8: 默认 SQL 超时 30s, 防慢 SQL 锁链式阻塞
// 修复 memory feedback_no_long_sql_no_timeout 描述的"一条慢 SQL 撑满 MaxOpenConns=100 锁全库"问题
const defaultQueryTimeout = 30 * time.Second

func writeDatabaseError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	log.Printf("database query failed: %v", err)
	writeError(w, http.StatusInternalServerError, "database query failed")
	return true
}

// rowsWithCancel v1.74.8: 包 *sql.Rows 给 Close() 加确定性 ctx cancel
// 替代 runtime.SetFinalizer (Codex 二审 P1: 不可靠, 累积 timer/资源)
// caller 代码 0 改动: Go 字段提升 rows.Next/Scan/Err 仍走 embedded *sql.Rows
// caller defer rows.Close() 触发 Close → 先 cancel ctx → 再 *sql.Rows.Close
type rowsWithCancel struct {
	*sql.Rows
	cancel context.CancelFunc
}

// Close 覆盖 *sql.Rows.Close: 先 cancel ctx 释放 timer, 再调原 Close
func (r *rowsWithCancel) Close() error {
	if r.cancel != nil {
		r.cancel()
	}
	return r.Rows.Close()
}

// queryRowsOrWriteError v1.74.8: 加 r 参数走 ctx + 30s timeout
// ctx 跟随 HTTP request lifecycle (client 断开 → ctx cancel → SQL 立即停),
// 30s 后 ctx 自动失效, 若 SQL 仍跑, rows.Next() 返 ErrContext, caller 走错误退出.
//
// cancel func 通过 rowsWithCancel 包装确定性 cleanup: caller `defer rows.Close()` 触发
// rowsWithCancel.Close() → cancel() + Rows.Close(), 杜绝 ctx tree leak.
func queryRowsOrWriteError(w http.ResponseWriter, r *http.Request, db *sql.DB, query string, args ...interface{}) (*rowsWithCancel, bool) {
	ctx, cancel := context.WithTimeout(r.Context(), defaultQueryTimeout)
	rows, err := db.QueryContext(ctx, query, args...)
	if writeDatabaseError(w, err) {
		cancel()
		return nil, false
	}
	return &rowsWithCancel{Rows: rows, cancel: cancel}, true
}
