package handler

// 客服-评论数据 明细查询 + 商品名改名(对照表, 不动原始)。
// 数据来自 op_customer_comment(由 import-comment 导入, 原始数据只读不改)。
// 客服逐条改商品名 → 存 comment_name_override(按 content_hash 关联), 可显示/可恢复;
// 每天 RPA 重导只更新 op_customer_comment, 改名对照独立保留。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ensureCommentOverrideTable 建改名对照表(幂等)。不动原始评论表。
func ensureCommentOverrideTable(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS comment_name_override (
		id BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键',
		content_hash CHAR(32) NOT NULL COMMENT '关联 op_customer_comment.content_hash',
		edited_name VARCHAR(255) NOT NULL DEFAULT '' COMMENT '客服改后的商品名',
		hidden TINYINT NOT NULL DEFAULT 0 COMMENT '客服删除标记(1=隐藏,原始数据仍保留)',
		edited_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '操作人',
		edited_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '操作时间',
		UNIQUE KEY uk_hash (content_hash)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='客服评论-改名/删除对照(不动RPA原始数据)'`)
	// 兼容已存在的旧表: 补 hidden 列(已存在则忽略报错)
	_, _ = db.Exec(`ALTER TABLE comment_name_override ADD COLUMN hidden TINYINT NOT NULL DEFAULT 0 COMMENT '客服删除标记(1=隐藏,原始保留)'`)
}

// CommentList GET /api/customer/comments — 评价明细 (筛选 platform/shop/date_from/date_to, 分页, 时间倒序)
func (h *DashboardHandler) CommentList(w http.ResponseWriter, r *http.Request) {
	ensureCommentOverrideTable(h.DB)
	q := r.URL.Query()
	platform := strings.TrimSpace(q.Get("platform"))
	shop := strings.TrimSpace(q.Get("shop"))
	dateFrom := strings.TrimSpace(q.Get("date_from"))
	dateTo := strings.TrimSpace(q.Get("date_to"))
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	var where []string
	var args []interface{}
	if platform != "" {
		where = append(where, "platform=?")
		args = append(args, platform)
	}
	if shop != "" {
		where = append(where, "shop_name=?")
		args = append(args, shop)
	}
	if dateFrom != "" {
		where = append(where, "comment_date>=?")
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		where = append(where, "comment_date<=?")
		args = append(args, dateTo)
	}
	// 默认排除被删除(隐藏)的; show_deleted=1(仅管理员前端有此开关)则只看被删的
	if q.Get("show_deleted") == "1" {
		where = append(where, "o.hidden=1")
	} else {
		where = append(where, "IFNULL(o.hidden,0)=0")
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM op_customer_comment c
		LEFT JOIN comment_name_override o ON o.content_hash=c.content_hash `+whereSQL, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "统计评价数失败: "+err.Error())
		return
	}

	// LEFT JOIN 改名对照: 有改后名显示改后名, 否则原始名; 同时带回原始名+是否已改+content_hash(给前端改名/恢复用)
	listSQL := `SELECT c.platform, c.shop_name,
		IFNULL(DATE_FORMAT(c.comment_date,'%Y-%m-%d'),'') AS d,
		c.order_no,
		IFNULL(NULLIF(o.edited_name,''), c.product_name) AS product_display,
		c.product_name AS product_original,
		CASE WHEN o.edited_name IS NOT NULL AND o.edited_name<>'' THEN 1 ELSE 0 END AS edited,
		c.comment_content, c.score,
		CASE WHEN c.platform='小红书' THEN '' ELSE c.score_raw END AS score_raw,
		c.content_hash
		FROM op_customer_comment c
		LEFT JOIN comment_name_override o ON o.content_hash=c.content_hash
		` + whereSQL + `
		ORDER BY c.comment_date DESC, c.id DESC LIMIT ? OFFSET ?`
	listArgs := append(append([]interface{}{}, args...), pageSize, (page-1)*pageSize)
	rows, err := h.DB.Query(listSQL, listArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询评价失败: "+err.Error())
		return
	}
	defer rows.Close()

	type item struct {
		Platform        string `json:"platform"`
		ShopName        string `json:"shopName"`
		Date            string `json:"date"`
		OrderNo         string `json:"orderNo"`
		Product         string `json:"product"`         // 显示名(改后或原始)
		ProductOriginal string `json:"productOriginal"` // RPA 原始名
		Edited          bool   `json:"edited"`          // 是否被客服改过
		Content         string `json:"content"`
		Score           *int   `json:"score"`
		ScoreText       string `json:"scoreText"`
		Hash            string `json:"hash"` // content_hash, 改名/恢复用
	}
	list := make([]item, 0, pageSize)
	for rows.Next() {
		var it item
		var score sql.NullInt64
		var edited int
		if err := rows.Scan(&it.Platform, &it.ShopName, &it.Date, &it.OrderNo,
			&it.Product, &it.ProductOriginal, &edited, &it.Content, &score, &it.ScoreText, &it.Hash); err != nil {
			continue
		}
		if score.Valid {
			s := int(score.Int64)
			it.Score = &s
		}
		it.Edited = edited == 1
		list = append(list, it)
	}
	writeJSON(w, map[string]interface{}{"list": list, "total": total, "page": page, "pageSize": pageSize})
}

// CommentRename POST /api/customer/comments/rename — 客服逐条改商品名(存对照表, 不动原始)
func (h *DashboardHandler) CommentRename(w http.ResponseWriter, r *http.Request) {
	ensureCommentOverrideTable(h.DB)
	var req struct {
		Hash string `json:"hash"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求解析失败: "+err.Error())
		return
	}
	req.Hash = strings.TrimSpace(req.Hash)
	req.Name = strings.TrimSpace(req.Name)
	if req.Hash == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "缺少评价标识或新名称")
		return
	}
	if len([]rune(req.Name)) > 255 {
		writeError(w, http.StatusBadRequest, "名称过长(最多255字)")
		return
	}
	if _, err := h.DB.Exec(`INSERT INTO comment_name_override (content_hash, edited_name)
		VALUES (?,?) ON DUPLICATE KEY UPDATE edited_name=VALUES(edited_name)`,
		req.Hash, req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, "保存改名失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// CommentRestore POST /api/customer/comments/restore — 恢复某条商品名到 RPA 原始(删对照记录)
func (h *DashboardHandler) CommentRestore(w http.ResponseWriter, r *http.Request) {
	ensureCommentOverrideTable(h.DB)
	var req struct {
		Hash string `json:"hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求解析失败: "+err.Error())
		return
	}
	req.Hash = strings.TrimSpace(req.Hash)
	if req.Hash == "" {
		writeError(w, http.StatusBadRequest, "缺少评价标识")
		return
	}
	if _, err := h.DB.Exec(`DELETE FROM comment_name_override WHERE content_hash=?`, req.Hash); err != nil {
		writeError(w, http.StatusInternalServerError, "恢复失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// CommentDelete POST /api/customer/comments/delete — 客服软删除一条评价(隐藏, 原始数据保留在库)
func (h *DashboardHandler) CommentDelete(w http.ResponseWriter, r *http.Request) {
	ensureCommentOverrideTable(h.DB)
	var req struct {
		Hash string `json:"hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求解析失败: "+err.Error())
		return
	}
	req.Hash = strings.TrimSpace(req.Hash)
	if req.Hash == "" {
		writeError(w, http.StatusBadRequest, "缺少评价标识")
		return
	}
	// 只标记 hidden=1, 不动 edited_name; 原始 op_customer_comment 完全不碰
	if _, err := h.DB.Exec(`INSERT INTO comment_name_override (content_hash, hidden)
		VALUES (?,1) ON DUPLICATE KEY UPDATE hidden=1`, req.Hash); err != nil {
		writeError(w, http.StatusInternalServerError, "删除失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// CommentUndelete POST /api/customer/comments/undelete — 恢复被删除的评价(仅管理员; 清隐藏标记, 保留改名)
func (h *DashboardHandler) CommentUndelete(w http.ResponseWriter, r *http.Request) {
	ensureCommentOverrideTable(h.DB)
	var req struct {
		Hash string `json:"hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求解析失败: "+err.Error())
		return
	}
	req.Hash = strings.TrimSpace(req.Hash)
	if req.Hash == "" {
		writeError(w, http.StatusBadRequest, "缺少评价标识")
		return
	}
	if _, err := h.DB.Exec(`UPDATE comment_name_override SET hidden=0 WHERE content_hash=?`, req.Hash); err != nil {
		writeError(w, http.StatusInternalServerError, "恢复失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// CommentOptions GET /api/customer/comment-options — 平台 + 店铺下拉选项 (店铺可按平台联动过滤)
func (h *DashboardHandler) CommentOptions(w http.ResponseWriter, r *http.Request) {
	platform := strings.TrimSpace(r.URL.Query().Get("platform"))

	platforms := make([]string, 0, 8)
	if prows, err := h.DB.Query("SELECT DISTINCT platform FROM op_customer_comment WHERE platform<>'' ORDER BY platform"); err == nil {
		defer prows.Close()
		for prows.Next() {
			var p string
			if prows.Scan(&p) == nil {
				platforms = append(platforms, p)
			}
		}
	}

	shopSQL := "SELECT DISTINCT shop_name FROM op_customer_comment WHERE shop_name<>''"
	var shopArgs []interface{}
	if platform != "" {
		shopSQL += " AND platform=?"
		shopArgs = append(shopArgs, platform)
	}
	shopSQL += " ORDER BY shop_name"
	shops := make([]string, 0, 32)
	if srows, err := h.DB.Query(shopSQL, shopArgs...); err == nil {
		defer srows.Close()
		for srows.Next() {
			var s string
			if srows.Scan(&s) == nil {
				shops = append(shops, s)
			}
		}
	}
	writeJSON(w, map[string]interface{}{"platforms": platforms, "shops": shops})
}

// CommentExport GET /api/customer/comments/export — 按当前筛选导出评价明细为 xlsx (商品名用改后显示名)
func (h *DashboardHandler) CommentExport(w http.ResponseWriter, r *http.Request) {
	ensureCommentOverrideTable(h.DB)
	q := r.URL.Query()
	platform := strings.TrimSpace(q.Get("platform"))
	shop := strings.TrimSpace(q.Get("shop"))
	dateFrom := strings.TrimSpace(q.Get("date_from"))
	dateTo := strings.TrimSpace(q.Get("date_to"))

	var where []string
	var args []interface{}
	if platform != "" {
		where = append(where, "platform=?")
		args = append(args, platform)
	}
	if shop != "" {
		where = append(where, "shop_name=?")
		args = append(args, shop)
	}
	if dateFrom != "" {
		where = append(where, "comment_date>=?")
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		where = append(where, "comment_date<=?")
		args = append(args, dateTo)
	}
	where = append(where, "IFNULL(o.hidden,0)=0")
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	rows, err := h.DB.Query(`SELECT c.platform, c.shop_name,
		IFNULL(DATE_FORMAT(c.comment_date,'%Y-%m-%d'),''), c.order_no,
		c.product_name AS product_original,
		IFNULL(o.edited_name,'') AS product_edited,
		c.comment_content,
		CASE WHEN c.platform='小红书' THEN '' ELSE c.score_raw END AS score_raw
		FROM op_customer_comment c
		LEFT JOIN comment_name_override o ON o.content_hash=c.content_hash
		`+whereSQL+` ORDER BY c.comment_date DESC, c.id DESC`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "导出查询失败: "+err.Error())
		return
	}
	defer rows.Close()

	xf := excelize.NewFile()
	sheet := "评论数据"
	xf.SetSheetName(xf.GetSheetName(0), sheet)
	headers := []string{"平台", "店铺", "时间", "订单编号", "商品名称", "客服改后名", "评价内容", "评分"}
	for i, hh := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		xf.SetCellValue(sheet, col+"1", hh)
	}
	rn := 2
	for rows.Next() {
		var pf, sp, date, orderNo, productOrig, productEdited, content, scoreText string
		if err := rows.Scan(&pf, &sp, &date, &orderNo, &productOrig, &productEdited, &content, &scoreText); err != nil {
			continue
		}
		n := strconv.Itoa(rn)
		xf.SetCellValue(sheet, "A"+n, pf)
		xf.SetCellValue(sheet, "B"+n, sp)
		xf.SetCellValue(sheet, "C"+n, date)
		xf.SetCellValue(sheet, "D"+n, orderNo) // 字符串, 防长订单号被 Excel 当数字科学计数
		xf.SetCellValue(sheet, "E"+n, productOrig)
		xf.SetCellValue(sheet, "F"+n, productEdited)
		xf.SetCellValue(sheet, "G"+n, content)
		xf.SetCellValue(sheet, "H"+n, scoreText)
		rn++
	}
	xf.SetColWidth(sheet, "A", "A", 10)
	xf.SetColWidth(sheet, "B", "B", 24)
	xf.SetColWidth(sheet, "C", "C", 12)
	xf.SetColWidth(sheet, "D", "D", 26)
	xf.SetColWidth(sheet, "E", "E", 40)
	xf.SetColWidth(sheet, "F", "F", 30)
	xf.SetColWidth(sheet, "G", "G", 50)
	xf.SetColWidth(sheet, "H", "H", 8)

	name := "评论数据"
	if platform != "" {
		name += "_" + platform
	}
	if shop != "" {
		name += "_" + shop
	}
	filename := name + ".xlsx"
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename*=UTF-8''%s`, urlEscape(filename)))
	_ = xf.Write(w)
}
