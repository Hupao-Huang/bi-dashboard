package handler

// 用友批量出库 / 批次转换 防重复提交 (idempotency)。
//
// 背景 (2026-06-10 跑哥踩坑): 一次点击在用友生成了重复出库/转换单据。根因是
//   ① 大批量执行耗时 > 服务器 WriteTimeout(120s) → 连接被掐断
//   ② 后端循环没接 r.Context() → 客户端断了仍继续往用友写单据
//   ③ 前端收到断连 → 弹"网络错误,执行失败"(其实后台已写成功) → 用户重发整批
//   ④ 转换/出库单【没有幂等键】→ 用友照单全收 → 重复数据 (不可逆)
//
// 这里提供共享的"提交流水"防重: 每建成一张用友单据就按内容指纹落一条流水,
// 同一笔在 ybDupWindowMin 分钟内再次提交时直接跳过, 不重复建单。配合
// handler 层清写超时 + ctx 取消 + 前端诚实文案, 四层兜住"点一次变多次"。

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
)

// ybDupWindowMin 防重窗口 (分钟): 同一笔转换/出库在此窗口内已提交过则跳过。
// 覆盖"超时假报失败→用户隔几十秒重发"的典型场景, 又不至于长到挡住正常的二次业务。
const ybDupWindowMin = 10

// ybFingerprint 把若干字段拼成内容指纹 (md5)。顺序敏感, 调用方保证字段顺序一致。
func ybFingerprint(parts ...string) string {
	sum := md5.Sum([]byte(strings.Join(parts, "\x1f"))) // 用不可见分隔符避免字段拼接歧义
	return hex.EncodeToString(sum[:])
}

// ybEnsureSubmitLog 幂等建"提交流水"表。失败不阻断执行 (降级=无防重但仍可出库)。
func (h *DashboardHandler) ybEnsureSubmitLog() {
	if h.DB == nil {
		return
	}
	_, _ = h.DB.Exec(`CREATE TABLE IF NOT EXISTS yonbip_submit_log (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		fingerprint CHAR(32) NOT NULL COMMENT '提交内容指纹(md5),同指纹短时间内视为重复提交',
		kind VARCHAR(16) NOT NULL DEFAULT '' COMMENT '类型:conv批次状态转换/conv_out出库内批次转换/out其他出库单',
		doc_code VARCHAR(64) NOT NULL DEFAULT '' COMMENT '用友返回单号',
		vouchdate VARCHAR(20) NOT NULL DEFAULT '' COMMENT '单据日期',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '提交时间',
		KEY idx_fp_time (fingerprint, created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用友批量出库/转换提交流水(防重复提交:同一笔10分钟内不重复建单)'`)
}

// ybRecentSubmit 查指纹在防重窗口内是否已提交过, 返回 (是否重复, 已有单号)。
// 查不到 / 查询出错都返回 false (降级放行, 靠 ctx 取消 + 前端文案兜底, 宁可不误挡)。
func (h *DashboardHandler) ybRecentSubmit(fp string) (bool, string) {
	if h.DB == nil {
		return false, ""
	}
	q := fmt.Sprintf(`SELECT COALESCE(doc_code,'') FROM yonbip_submit_log
		WHERE fingerprint=? AND created_at >= (NOW() - INTERVAL %d MINUTE)
		ORDER BY id DESC LIMIT 1`, ybDupWindowMin)
	var doc string
	if err := h.DB.QueryRow(q, fp).Scan(&doc); err != nil {
		return false, ""
	}
	return true, doc
}

// ybRecordSubmit 单据在用友建成后立即落流水 (即便后续审核失败/连接断, 重发也会被跳过)。
func (h *DashboardHandler) ybRecordSubmit(fp, kind, docCode, vouchdate string) {
	if h.DB == nil {
		return
	}
	_, _ = h.DB.Exec(
		`INSERT INTO yonbip_submit_log (fingerprint, kind, doc_code, vouchdate) VALUES (?,?,?,?)`,
		fp, kind, docCode, vouchdate)
}
