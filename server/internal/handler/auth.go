package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"image/color"
	"image/png"
	"log"
	"math"
	mrand "math/rand"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName    = "bi_dashboard_session"
	defaultAdminUsername = "admin"
	defaultAdminPassword = "Admin@123456"
	maxLoginAttempts     = 5
	lockDuration         = 5 * time.Minute
	captchaExpiry        = 5 * time.Minute
	shortSessionDuration = 8 * time.Hour
	idleTimeout          = 2 * time.Hour
)

var sessionDuration = 7 * 24 * time.Hour

type dingtalkPendingUser struct {
	Nick       string
	UnionId    string
	OpenId     string
	Mobile     string
	Department string
	Expires    time.Time
}

var dingtalkPendingUsers = sync.Map{}

// ==================== 验证码 ====================

type captchaEntry struct {
	targetX   int
	expiresAt time.Time
	verified  bool // preVerify 通过后置 true，阻止同一 captcha 被反复爆破
}

var (
	captchaStore = make(map[string]captchaEntry)
	captchaMu    sync.Mutex
)

const (
	captchaW  = 320
	captchaH  = 160
	pieceSize = 44
	piecePad  = 6 // 凸起大小
	tolerance = 5 // 验证容差(px)
)

func generateCaptchaID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// 生成渐变+噪点背景
func generateBackground() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, captchaW, captchaH))

	// 随机渐变色
	palettes := [][2]color.RGBA{
		{{70, 130, 180, 255}, {176, 224, 230, 255}},
		{{147, 112, 219, 255}, {216, 191, 216, 255}},
		{{60, 179, 113, 255}, {144, 238, 144, 255}},
		{{205, 133, 63, 255}, {245, 222, 179, 255}},
		{{100, 149, 237, 255}, {173, 216, 230, 255}},
		{{199, 97, 20, 255}, {255, 200, 124, 255}},
	}
	pal := palettes[mrand.Intn(len(palettes))]

	for y := 0; y < captchaH; y++ {
		t := float64(y) / float64(captchaH)
		for x := 0; x < captchaW; x++ {
			tx := float64(x) / float64(captchaW)
			blend := (t + tx) / 2
			r := uint8(float64(pal[0].R)*(1-blend) + float64(pal[1].R)*blend)
			g := uint8(float64(pal[0].G)*(1-blend) + float64(pal[1].G)*blend)
			b := uint8(float64(pal[0].B)*(1-blend) + float64(pal[1].B)*blend)
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}

	// 随机色块
	for i := 0; i < 8; i++ {
		cx, cy := mrand.Intn(captchaW), mrand.Intn(captchaH)
		radius := 15 + mrand.Intn(30)
		c := color.RGBA{uint8(mrand.Intn(255)), uint8(mrand.Intn(255)), uint8(mrand.Intn(255)), 40}
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if dx*dx+dy*dy <= radius*radius {
					px, py := cx+dx, cy+dy
					if px >= 0 && px < captchaW && py >= 0 && py < captchaH {
						orig := img.RGBAAt(px, py)
						img.Set(px, py, blendColor(orig, c))
					}
				}
			}
		}
	}

	// 干扰线
	for i := 0; i < 3; i++ {
		lc := color.RGBA{uint8(mrand.Intn(200)), uint8(mrand.Intn(200)), uint8(mrand.Intn(200)), 60}
		x1, y1 := mrand.Intn(captchaW), mrand.Intn(captchaH)
		x2, y2 := mrand.Intn(captchaW), mrand.Intn(captchaH)
		drawLine(img, x1, y1, x2, y2, lc)
	}

	return img
}

func blendColor(bg color.RGBA, fg color.RGBA) color.RGBA {
	a := float64(fg.A) / 255
	return color.RGBA{
		R: uint8(float64(bg.R)*(1-a) + float64(fg.R)*a),
		G: uint8(float64(bg.G)*(1-a) + float64(fg.G)*a),
		B: uint8(float64(bg.B)*(1-a) + float64(fg.B)*a),
		A: 255,
	}
}

func drawLine(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	dx := x2 - x1
	dy := y2 - y1
	steps := int(math.Max(math.Abs(float64(dx)), math.Abs(float64(dy))))
	if steps == 0 {
		return
	}
	xInc := float64(dx) / float64(steps)
	yInc := float64(dy) / float64(steps)
	x, y := float64(x1), float64(y1)
	for i := 0; i <= steps; i++ {
		ix, iy := int(x), int(y)
		if ix >= 0 && ix < img.Bounds().Dx() && iy >= 0 && iy < img.Bounds().Dy() {
			orig := img.RGBAAt(ix, iy)
			img.Set(ix, iy, blendColor(orig, c))
		}
		x += xInc
		y += yInc
	}
}

// 判断点是否在拼图块形状内（正方形+右侧和底部凸起）
func inPieceShape(px, py, cx, cy int) bool {
	s := pieceSize
	p := piecePad
	// 主体方块
	if px >= cx && px < cx+s && py >= cy && py < cy+s {
		return true
	}
	// 右侧凸起（半圆）
	rcx, rcy := cx+s, cy+s/2
	if (px-rcx)*(px-rcx)+(py-rcy)*(py-rcy) <= p*p {
		return true
	}
	// 底部凸起（半圆）
	bcx, bcy := cx+s/2, cy+s
	if (px-bcx)*(px-bcx)+(py-bcy)*(py-bcy) <= p*p {
		return true
	}
	return false
}

func (h *DashboardHandler) GetCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	captchaMu.Lock()
	now := time.Now()
	for id, entry := range captchaStore {
		if now.After(entry.expiresAt) {
			delete(captchaStore, id)
		}
	}
	captchaMu.Unlock()

	bg := generateBackground()

	// 随机拼图位置
	targetX := 80 + mrand.Intn(captchaW-pieceSize-100)
	targetY := 20 + mrand.Intn(captchaH-pieceSize-40)

	// 提取拼图块
	totalW := pieceSize + piecePad
	totalH := pieceSize + piecePad
	piece := image.NewRGBA(image.Rect(0, 0, totalW, totalH))

	for py := 0; py < totalH; py++ {
		for px := 0; px < totalW; px++ {
			if inPieceShape(targetX+px, targetY+py, targetX, targetY) {
				piece.Set(px, py, bg.RGBAAt(targetX+px, targetY+py))
			}
		}
	}

	// 给拼图块加边框
	for py := 0; py < totalH; py++ {
		for px := 0; px < totalW; px++ {
			if !inPieceShape(targetX+px, targetY+py, targetX, targetY) {
				continue
			}
			// 检查相邻像素是否在形状外（边缘检测）
			isEdge := false
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := px+d[0], py+d[1]
				if !inPieceShape(targetX+nx, targetY+ny, targetX, targetY) {
					isEdge = true
					break
				}
			}
			if isEdge {
				piece.Set(px, py, color.RGBA{255, 255, 255, 200})
			}
		}
	}

	// 在背景上挖洞（半透明灰色覆盖）
	bgWithHole := image.NewRGBA(bg.Bounds())
	copy(bgWithHole.Pix, bg.Pix)
	for py := 0; py < totalH; py++ {
		for px := 0; px < totalW; px++ {
			if inPieceShape(targetX+px, targetY+py, targetX, targetY) {
				orig := bgWithHole.RGBAAt(targetX+px, targetY+py)
				dark := color.RGBA{
					R: uint8(float64(orig.R) * 0.4),
					G: uint8(float64(orig.G) * 0.4),
					B: uint8(float64(orig.B) * 0.4),
					A: 255,
				}
				bgWithHole.Set(targetX+px, targetY+py, dark)
			}
		}
	}

	// 编码为base64
	var bgBuf, pieceBuf bytes.Buffer
	png.Encode(&bgBuf, bgWithHole)
	png.Encode(&pieceBuf, piece)

	bgB64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(bgBuf.Bytes())
	pieceB64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pieceBuf.Bytes())

	id := generateCaptchaID()
	captchaMu.Lock()
	captchaStore[id] = captchaEntry{targetX: targetX, expiresAt: time.Now().Add(captchaExpiry)}
	captchaMu.Unlock()

	writeJSON(w, map[string]interface{}{
		"id":    id,
		"bg":    bgB64,
		"piece": pieceB64,
		"y":     targetY,
	})
}

// verifyCaptcha login 消耗 captchaId：必须已经通过 preVerify 才放行
func verifyCaptcha(id string, answerX int) bool {
	captchaMu.Lock()
	defer captchaMu.Unlock()
	entry, ok := captchaStore[id]
	if !ok {
		return false
	}
	delete(captchaStore, id)
	if time.Now().After(entry.expiresAt) {
		return false
	}
	if !entry.verified {
		return false
	}
	diff := entry.targetX - answerX
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

// preVerifyCaptcha 预验证：失败立即销毁 captchaId，成功则 mark verified（等 login 再消耗）
// 这样同一 captchaId 无法被反复爆破：猜错一次就作废
func preVerifyCaptcha(id string, answerX int) bool {
	captchaMu.Lock()
	defer captchaMu.Unlock()
	entry, ok := captchaStore[id]
	if !ok {
		return false
	}
	if time.Now().After(entry.expiresAt) {
		delete(captchaStore, id)
		return false
	}
	diff := entry.targetX - answerX
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		delete(captchaStore, id) // 防爆破：失败即销毁
		return false
	}
	entry.verified = true
	captchaStore[id] = entry
	return true
}

// VerifyCaptchaOnly 前端滑块验证接口（不消耗captcha，仅检查位置）
func (h *DashboardHandler) VerifyCaptchaOnly(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req struct {
		CaptchaID     string `json:"captchaId"`
		CaptchaAnswer int    `json:"captchaAnswer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "bad request")
		return
	}
	if preVerifyCaptcha(req.CaptchaID, req.CaptchaAnswer) {
		writeJSON(w, map[string]interface{}{"code": 200, "msg": "ok"})
	} else {
		writeError(w, 400, "验证失败")
	}
}

// ==================== 登录频率限制 ====================

type loginAttempt struct {
	count    int
	lockedAt time.Time
}

var (
	loginAttempts = make(map[string]*loginAttempt)
	loginMu       sync.Mutex
)

func checkLoginLock(ip string) (bool, int) {
	loginMu.Lock()
	defer loginMu.Unlock()
	attempt, ok := loginAttempts[ip]
	if !ok {
		return false, 0
	}
	if !attempt.lockedAt.IsZero() && time.Now().Before(attempt.lockedAt.Add(lockDuration)) {
		remaining := int(time.Until(attempt.lockedAt.Add(lockDuration)).Minutes()) + 1
		return true, remaining
	}
	if !attempt.lockedAt.IsZero() && time.Now().After(attempt.lockedAt.Add(lockDuration)) {
		delete(loginAttempts, ip)
		return false, 0
	}
	return false, 0
}

func recordLoginFailure(ip string) int {
	loginMu.Lock()
	defer loginMu.Unlock()
	attempt, ok := loginAttempts[ip]
	if !ok {
		attempt = &loginAttempt{}
		loginAttempts[ip] = attempt
	}
	attempt.count++
	if attempt.count >= maxLoginAttempts {
		attempt.lockedAt = time.Now()
		return 0
	}
	return maxLoginAttempts - attempt.count
}

func clearLoginFailure(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	delete(loginAttempts, ip)
}

type authContextKey string

const currentAuthPayloadKey authContextKey = "currentAuthPayload"

type authPayload struct {
	User               authUser       `json:"user"`
	Roles              []string       `json:"roles"`
	Permissions        []string       `json:"permissions"`
	DataScopes         authDataScopes `json:"dataScopes"`
	IsSuperAdmin       bool           `json:"isSuperAdmin"`
	MustChangePassword bool           `json:"mustChangePassword"`
}

type authUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	RealName string `json:"realName"`
}

type authDataScopes struct {
	Depts      []string `json:"depts"`
	Platforms  []string `json:"platforms"`
	Shops      []string `json:"shops"`
	Warehouses []string `json:"warehouses"`
	Domains    []string `json:"domains"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type permissionSeed struct {
	Code string
	Name string
	Type string
}

type roleSeed struct {
	Code        string
	Name        string
	Description string
}

var permissionSeeds = []permissionSeed{
	{Code: "overview:view", Name: "综合看板", Type: "page"},
	{Code: "brand:view", Name: "品牌中心", Type: "menu"},
	{Code: "ecommerce:view", Name: "电商部门", Type: "menu"},
	{Code: "ecommerce.store_preview:view", Name: "电商-店铺数据预览", Type: "page"},
	{Code: "ecommerce.store_dashboard:view", Name: "电商-店铺看板", Type: "page"},
	{Code: "ecommerce.product_dashboard:view", Name: "电商-货品看板", Type: "page"},
	{Code: "ecommerce.marketing_cost:view", Name: "电商-营销费用", Type: "page"},
	{Code: "social:view", Name: "社媒部门", Type: "menu"},
	{Code: "social.store_preview:view", Name: "社媒-店铺数据预览", Type: "page"},
	{Code: "social.store_dashboard:view", Name: "社媒-店铺看板", Type: "page"},
	{Code: "social.product_dashboard:view", Name: "社媒-货品看板", Type: "page"},
	{Code: "social.feigua:view", Name: "社媒-飞瓜看板", Type: "page"},
	{Code: "social.marketing:view", Name: "社媒-营销看板", Type: "page"},
	{Code: "offline:view", Name: "线下部门", Type: "menu"},
	{Code: "offline.store_preview:view", Name: "线下-店铺数据预览", Type: "page"},
	{Code: "offline.store_dashboard:view", Name: "线下-店铺看板", Type: "page"},
	{Code: "offline.product_dashboard:view", Name: "线下-货品看板", Type: "page"},
	{Code: "offline.high_value_customers:view", Name: "线下-高价值客户", Type: "page"},
	{Code: "offline.turnover_expiry:view", Name: "线下-周转率及临期", Type: "page"},
	{Code: "offline.ka_monthly:view", Name: "线下-KA月度统计", Type: "page"},
	{Code: "distribution:view", Name: "分销部门", Type: "menu"},
	{Code: "distribution.store_preview:view", Name: "分销-店铺数据预览", Type: "page"},
	{Code: "distribution.store_dashboard:view", Name: "分销-店铺看板", Type: "page"},
	{Code: "distribution.product_dashboard:view", Name: "分销-货品看板", Type: "page"},
	{Code: "finance:view", Name: "财务部门", Type: "menu"},
	{Code: "finance.overview:view", Name: "财务-利润总览", Type: "page"},
	{Code: "finance.department_profit:view", Name: "财务-部门利润分析", Type: "page"},
	{Code: "finance.monthly_profit:view", Name: "财务-月度利润统计", Type: "page"},
	{Code: "finance.product_profit:view", Name: "财务-产品利润统计", Type: "page"},
	{Code: "finance.expense:view", Name: "财务-费控管理", Type: "page"},
	{Code: "finance.report:view", Name: "财务-财务报表", Type: "page"},
	{Code: "finance.report:import", Name: "财务-财务报表导入", Type: "action"},
	{Code: "customer:view", Name: "客服部门", Type: "menu"},
	{Code: "customer.overview:view", Name: "客服-客服总览", Type: "page"},
	{Code: "supply_chain:view", Name: "供应链管理", Type: "menu"},
	{Code: "supply_chain.plan_dashboard:view", Name: "供应链-计划看板", Type: "page"},
	{Code: "supply_chain.inventory_warning:view", Name: "供应链-库存预警", Type: "page"},
	{Code: "supply_chain.logistics_analysis:view", Name: "供应链-快递仓储分析", Type: "page"},
	{Code: "supply_chain.daily_alerts:view", Name: "供应链-每日预警", Type: "page"},
	{Code: "supply_chain.monthly_billing:view", Name: "供应链-月度账单分析", Type: "page"},
	{Code: "profit:view", Name: "查看利润", Type: "field"},
	{Code: "cost:view", Name: "查看成本", Type: "field"},
	{Code: "gross_margin:view", Name: "查看毛利率", Type: "field"},
	{Code: "user.manage", Name: "用户管理", Type: "action"},
	{Code: "role.manage", Name: "角色管理", Type: "action"},
	{Code: "feedback.manage", Name: "反馈管理", Type: "action"},
	{Code: "notice.manage", Name: "公告管理", Type: "action"},
	{Code: "channel.manage", Name: "渠道管理", Type: "action"},
}

var roleSeeds = []roleSeed{
	{Code: "super_admin", Name: "超级管理员", Description: "拥有全部权限"},
	{Code: "management", Name: "管理层", Description: "查看全局经营数据"},
	{Code: "dept_manager", Name: "部门负责人", Description: "按部门查看业务数据"},
	{Code: "operator", Name: "运营", Description: "查看分配的平台和店铺"},
	{Code: "finance", Name: "财务", Description: "查看财务与利润数据"},
	{Code: "supply_chain", Name: "供应链", Description: "查看供应链相关数据"},
}

var roleDefaultPermissions = map[string][]string{
	"management": {
		"overview:view",
		"brand:view",
		"ecommerce:view", "ecommerce.store_preview:view", "ecommerce.store_dashboard:view", "ecommerce.product_dashboard:view", "ecommerce.marketing_cost:view",
		"social:view", "social.store_preview:view", "social.store_dashboard:view", "social.product_dashboard:view", "social.feigua:view",
		"offline:view", "offline.store_preview:view", "offline.store_dashboard:view", "offline.product_dashboard:view", "offline.high_value_customers:view", "offline.turnover_expiry:view", "offline.ka_monthly:view",
		"distribution:view", "distribution.store_preview:view", "distribution.store_dashboard:view", "distribution.product_dashboard:view",
		"finance:view", "finance.overview:view", "finance.department_profit:view", "finance.monthly_profit:view", "finance.product_profit:view", "finance.expense:view", "finance.report:view",
		"customer:view", "customer.overview:view",
		"supply_chain:view", "supply_chain.plan_dashboard:view", "supply_chain.inventory_warning:view", "supply_chain.logistics_analysis:view", "supply_chain.daily_alerts:view", "supply_chain.monthly_billing:view",
		"profit:view", "cost:view", "gross_margin:view",
	},
	"dept_manager": {
		"overview:view",
		"brand:view",
		"ecommerce:view", "ecommerce.store_preview:view", "ecommerce.store_dashboard:view", "ecommerce.product_dashboard:view", "ecommerce.marketing_cost:view",
		"social:view", "social.store_preview:view", "social.store_dashboard:view", "social.product_dashboard:view", "social.feigua:view",
		"offline:view", "offline.store_preview:view", "offline.store_dashboard:view", "offline.product_dashboard:view", "offline.high_value_customers:view", "offline.turnover_expiry:view", "offline.ka_monthly:view",
		"distribution:view", "distribution.store_preview:view", "distribution.store_dashboard:view", "distribution.product_dashboard:view",
		"customer:view", "customer.overview:view",
	},
	"operator": {
		"brand:view",
		"ecommerce:view", "ecommerce.store_preview:view", "ecommerce.store_dashboard:view", "ecommerce.product_dashboard:view", "ecommerce.marketing_cost:view",
		"social:view", "social.store_preview:view", "social.store_dashboard:view", "social.product_dashboard:view", "social.feigua:view",
		"offline:view", "offline.store_preview:view", "offline.store_dashboard:view", "offline.product_dashboard:view", "offline.high_value_customers:view", "offline.turnover_expiry:view", "offline.ka_monthly:view",
		"distribution:view", "distribution.store_preview:view", "distribution.store_dashboard:view", "distribution.product_dashboard:view",
		"customer:view", "customer.overview:view",
	},
	"finance": {
		"overview:view",
		"brand:view",
		"finance:view", "finance.overview:view", "finance.department_profit:view", "finance.monthly_profit:view", "finance.product_profit:view", "finance.expense:view", "finance.report:view", "finance.report:import",
		"profit:view", "cost:view", "gross_margin:view",
	},
	"supply_chain": {
		"brand:view",
		"supply_chain:view", "supply_chain.plan_dashboard:view", "supply_chain.inventory_warning:view", "supply_chain.logistics_analysis:view", "supply_chain.daily_alerts:view", "supply_chain.monthly_billing:view",
	},
}

func allPermissionCodes() []string {
	codes := make([]string, 0, len(permissionSeeds))
	for _, seed := range permissionSeeds {
		codes = append(codes, seed.Code)
	}
	return codes
}

func EnsureAuthSchemaAndSeed(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			username VARCHAR(64) NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			real_name VARCHAR(64) DEFAULT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'active',
			last_login_at DATETIME DEFAULT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_username (username)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统用户'`,
		`CREATE TABLE IF NOT EXISTS roles (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			code VARCHAR(64) NOT NULL,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(255) DEFAULT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_role_code (code)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统角色'`,
		`CREATE TABLE IF NOT EXISTS permissions (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			code VARCHAR(128) NOT NULL,
			name VARCHAR(100) NOT NULL,
			type VARCHAR(20) NOT NULL DEFAULT 'page',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_permission_code (code)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='权限点'`,
		`CREATE TABLE IF NOT EXISTS user_roles (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			role_id BIGINT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_user_role (user_id, role_id),
			KEY idx_user_roles_user_id (user_id),
			KEY idx_user_roles_role_id (role_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户角色关联'`,
		`CREATE TABLE IF NOT EXISTS role_permissions (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			role_id BIGINT NOT NULL,
			permission_id BIGINT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_role_permission (role_id, permission_id),
			KEY idx_role_permissions_role_id (role_id),
			KEY idx_role_permissions_permission_id (permission_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='角色权限关联'`,
		`CREATE TABLE IF NOT EXISTS data_scopes (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			subject_type VARCHAR(16) NOT NULL,
			subject_id BIGINT NOT NULL,
			scope_type VARCHAR(20) NOT NULL,
			scope_value VARCHAR(128) NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_data_scope (subject_type, subject_id, scope_type, scope_value),
			KEY idx_data_scope_subject (subject_type, subject_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='数据权限范围'`,
		`CREATE TABLE IF NOT EXISTS user_sessions (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			token_hash CHAR(64) NOT NULL,
			expires_at DATETIME NOT NULL,
			last_active_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			ip VARCHAR(64) DEFAULT NULL,
			user_agent VARCHAR(255) DEFAULT NULL,
			UNIQUE KEY uk_token_hash (token_hash),
			KEY idx_user_sessions_user_id (user_id),
			KEY idx_user_sessions_expires_at (expires_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='登录会话'`,

		`CREATE TABLE IF NOT EXISTS feedback (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			username VARCHAR(64) NOT NULL,
			real_name VARCHAR(64) NOT NULL DEFAULT '',
			title VARCHAR(255) NOT NULL,
			content TEXT NOT NULL,
			page_url VARCHAR(500) DEFAULT '',
			attachments JSON DEFAULT NULL COMMENT '附件列表，JSON数组',
			status VARCHAR(20) NOT NULL DEFAULT 'pending' COMMENT 'pending/processing/resolved/closed',
			reply TEXT DEFAULT NULL COMMENT '管理员回复',
			replied_by VARCHAR(64) DEFAULT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			KEY idx_feedback_user_id (user_id),
			KEY idx_feedback_status (status),
			KEY idx_feedback_created_at (created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户反馈'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_live_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			anchor_name VARCHAR(100) DEFAULT '',
			anchor_id VARCHAR(100) DEFAULT '',
			start_time DATETIME DEFAULT NULL,
			end_time DATETIME DEFAULT NULL,
			duration_min DECIMAL(10,2) DEFAULT 0,
			exposure_uv INT DEFAULT 0 COMMENT '曝光人数',
			watch_uv INT DEFAULT 0 COMMENT '观看人数',
			watch_pv INT DEFAULT 0 COMMENT '观看次数',
			max_online INT DEFAULT 0 COMMENT '最高在线',
			avg_online INT DEFAULT 0 COMMENT '平均在线',
			avg_watch_min DECIMAL(10,2) DEFAULT 0 COMMENT '人均观看时长(分)',
			comments INT DEFAULT 0,
			new_fans INT DEFAULT 0,
			product_count INT DEFAULT 0,
			product_exposure_uv INT DEFAULT 0,
			product_click_uv INT DEFAULT 0,
			order_count INT DEFAULT 0,
			order_amount DECIMAL(14,2) DEFAULT 0,
			pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '用户支付金额',
			pay_per_hour DECIMAL(14,2) DEFAULT 0,
			pay_qty INT DEFAULT 0,
			pay_uv INT DEFAULT 0,
			refund_count INT DEFAULT 0,
			refund_amount DECIMAL(14,2) DEFAULT 0,
			ad_cost_bindshop DECIMAL(14,2) DEFAULT 0 COMMENT '投放消耗(店铺绑定)',
			ad_cost_invested DECIMAL(14,2) DEFAULT 0 COMMENT '投放消耗(店铺被投)',
			net_order_amount DECIMAL(14,2) DEFAULT 0 COMMENT '净成交金额',
			net_order_count INT DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0 COMMENT '1小时退款率',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			KEY idx_douyin_live_date (stat_date),
			KEY idx_douyin_live_shop (shop_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营直播数据'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_goods_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			product_id VARCHAR(64) NOT NULL,
			product_name VARCHAR(255) DEFAULT '',
			explain_count INT DEFAULT 0 COMMENT '讲解次数',
			live_price VARCHAR(50) DEFAULT '',
			pay_amount DECIMAL(14,2) DEFAULT 0,
			pay_qty INT DEFAULT 0,
			presale_count INT DEFAULT 0,
			click_uv INT DEFAULT 0,
			click_rate DECIMAL(8,4) DEFAULT 0,
			conv_rate DECIMAL(8,4) DEFAULT 0,
			cpm_pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '千次曝光支付金额',
			pre_refund_count INT DEFAULT 0,
			pre_refund_amount DECIMAL(14,2) DEFAULT 0,
			post_refund_count INT DEFAULT 0,
			post_refund_amount DECIMAL(14,2) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_douyin_goods (stat_date, shop_name, product_id),
			KEY idx_douyin_goods_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营商品数据'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_dist_product_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			account_name VARCHAR(200) NOT NULL COMMENT '投放账户(文件夹名)',
			product_id VARCHAR(64) NOT NULL,
			product_name VARCHAR(255) DEFAULT '',
			impressions INT DEFAULT 0,
			clicks INT DEFAULT 0,
			click_rate DECIMAL(8,4) DEFAULT 0,
			conv_rate DECIMAL(8,4) DEFAULT 0,
			cost DECIMAL(14,2) DEFAULT 0 COMMENT '整体消耗',
			pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '整体成交金额',
			roi DECIMAL(10,2) DEFAULT 0,
			order_cost DECIMAL(10,2) DEFAULT 0,
			user_pay_amount DECIMAL(14,2) DEFAULT 0,
			subsidy_amount DECIMAL(14,2) DEFAULT 0 COMMENT '平台补贴',
			net_roi DECIMAL(10,2) DEFAULT 0,
			net_amount DECIMAL(14,2) DEFAULT 0,
			net_order_cost DECIMAL(10,2) DEFAULT 0,
			net_settle_rate DECIMAL(8,4) DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_dist_product (stat_date, account_name, product_id),
			KEY idx_dist_product_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音分销推商品'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_dist_account_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			account_name VARCHAR(200) NOT NULL COMMENT '投放账户(文件夹名)',
			douyin_name VARCHAR(200) DEFAULT '',
			douyin_id VARCHAR(100) DEFAULT '',
			cost DECIMAL(14,2) DEFAULT 0,
			order_count INT DEFAULT 0,
			pay_amount DECIMAL(14,2) DEFAULT 0,
			roi DECIMAL(10,2) DEFAULT 0,
			order_cost DECIMAL(10,2) DEFAULT 0,
			user_pay_amount DECIMAL(14,2) DEFAULT 0,
			subsidy_amount DECIMAL(14,2) DEFAULT 0,
			net_roi DECIMAL(10,2) DEFAULT 0,
			net_amount DECIMAL(14,2) DEFAULT 0,
			net_settle_rate DECIMAL(8,4) DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_dist_account (stat_date, account_name, douyin_name),
			KEY idx_dist_account_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音分销推抖音号'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_channel_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			channel_name VARCHAR(100) NOT NULL COMMENT '渠道名(推荐feed/直播广场/同城等)',
			watch_ucnt INT DEFAULT 0 COMMENT '观看人数',
			watch_cnt INT DEFAULT 0 COMMENT '观看次数',
			avg_watch_duration DECIMAL(10,2) DEFAULT 0 COMMENT '平均观看时长(秒)',
			pay_amt DECIMAL(14,2) DEFAULT 0 COMMENT '成交金额',
			pay_cnt INT DEFAULT 0 COMMENT '成交人数',
			avg_pay_order_amt DECIMAL(10,2) DEFAULT 0 COMMENT '客单价',
			watch_pay_cnt_ratio DECIMAL(8,4) DEFAULT 0 COMMENT '观看-成交率',
			interact_watch_cnt_ratio DECIMAL(8,4) DEFAULT 0 COMMENT '互动-观看率',
			ad_costed_amt DECIMAL(14,2) DEFAULT 0 COMMENT '广告消耗',
			stat_cost DECIMAL(14,2) DEFAULT 0 COMMENT '统计消耗',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_douyin_channel (stat_date, shop_name, channel_name),
			KEY idx_douyin_channel_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营渠道分析'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_funnel_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			step_name VARCHAR(100) NOT NULL COMMENT '漏斗步骤名',
			step_value BIGINT DEFAULT 0 COMMENT '人数/金额',
			step_order INT DEFAULT 0 COMMENT '排序',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_douyin_funnel (stat_date, shop_name, step_name),
			KEY idx_douyin_funnel_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营转化漏斗'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_ad_live_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			douyin_name VARCHAR(200) DEFAULT '',
			douyin_id VARCHAR(100) DEFAULT '',
			net_roi DECIMAL(10,2) DEFAULT 0,
			net_amount DECIMAL(14,2) DEFAULT 0 COMMENT '净成交金额',
			net_order_count INT DEFAULT 0,
			net_order_cost DECIMAL(10,2) DEFAULT 0,
			user_pay_amount DECIMAL(14,2) DEFAULT 0,
			net_settle_rate DECIMAL(8,4) DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0,
			impressions INT DEFAULT 0 COMMENT '展示次数',
			clicks INT DEFAULT 0,
			click_rate DECIMAL(8,4) DEFAULT 0,
			conv_rate DECIMAL(8,4) DEFAULT 0,
			cost DECIMAL(14,2) DEFAULT 0 COMMENT '整体消耗',
			pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '整体成交金额',
			roi DECIMAL(10,2) DEFAULT 0,
			cpm DECIMAL(10,2) DEFAULT 0 COMMENT '千次展现费用',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_douyin_ad_live (stat_date, shop_name, douyin_name),
			KEY idx_douyin_ad_live_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营推广直播间画面'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_anchor_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			account VARCHAR(100) DEFAULT '' COMMENT '账号',
			anchor_name VARCHAR(100) NOT NULL,
			duration VARCHAR(50) DEFAULT '',
			pay_amount DECIMAL(14,2) DEFAULT 0 COMMENT '直播间用户支付金额',
			pay_per_hour DECIMAL(14,2) DEFAULT 0,
			max_online INT DEFAULT 0,
			avg_online INT DEFAULT 0,
			avg_item_price DECIMAL(10,2) DEFAULT 0 COMMENT '平均件单价',
			exposure_watch_rate VARCHAR(20) DEFAULT '' COMMENT '曝光-观看率',
			retention_rate VARCHAR(20) DEFAULT '' COMMENT '留存率',
			interact_rate VARCHAR(20) DEFAULT '' COMMENT '互动率',
			new_fans INT DEFAULT 0,
			fans_rate VARCHAR(20) DEFAULT '' COMMENT '关注率',
			new_group INT DEFAULT 0 COMMENT '新加团人数',
			uv_value DECIMAL(10,2) DEFAULT 0 COMMENT '单UV价值',
			pay_uv INT DEFAULT 0 COMMENT '成交人数',
			crowd_top3 VARCHAR(500) DEFAULT '' COMMENT '策略人群TOP3',
			gender VARCHAR(100) DEFAULT '' COMMENT '性别分布',
			age_top3 VARCHAR(500) DEFAULT '' COMMENT '年龄段TOP3',
			city_level_top3 VARCHAR(500) DEFAULT '' COMMENT '城市等级TOP3',
			region_top3 VARCHAR(500) DEFAULT '' COMMENT '地域分布TOP3',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_douyin_anchor (stat_date, shop_name, anchor_name),
			KEY idx_douyin_anchor_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营主播分析'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_ad_material_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			shop_name VARCHAR(100) NOT NULL,
			material_name VARCHAR(500) DEFAULT '',
			material_id VARCHAR(100) DEFAULT '',
			material_eval VARCHAR(50) DEFAULT '' COMMENT '素材评估',
			material_duration VARCHAR(50) DEFAULT '',
			material_source VARCHAR(100) DEFAULT '',
			net_roi DECIMAL(10,2) DEFAULT 0,
			net_amount DECIMAL(14,2) DEFAULT 0,
			net_order_count INT DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0,
			impressions INT DEFAULT 0,
			clicks INT DEFAULT 0,
			click_rate DECIMAL(8,4) DEFAULT 0,
			conv_rate DECIMAL(8,4) DEFAULT 0,
			cost DECIMAL(14,2) DEFAULT 0,
			pay_amount DECIMAL(14,2) DEFAULT 0,
			roi DECIMAL(10,2) DEFAULT 0,
			cpm DECIMAL(10,2) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			KEY idx_douyin_material_date (stat_date),
			KEY idx_douyin_material_shop (shop_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音自营推广视频素材'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_dist_material_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			account_name VARCHAR(200) NOT NULL,
			material_id VARCHAR(100) DEFAULT '',
			material_name VARCHAR(500) DEFAULT '',
			impressions INT DEFAULT 0,
			clicks INT DEFAULT 0,
			click_rate DECIMAL(8,4) DEFAULT 0,
			conv_rate DECIMAL(8,4) DEFAULT 0,
			cost DECIMAL(14,2) DEFAULT 0,
			order_count INT DEFAULT 0,
			pay_amount DECIMAL(14,2) DEFAULT 0,
			roi DECIMAL(10,2) DEFAULT 0,
			order_cost DECIMAL(10,2) DEFAULT 0,
			user_pay_amount DECIMAL(14,2) DEFAULT 0,
			cpm DECIMAL(10,2) DEFAULT 0,
			cpc DECIMAL(10,2) DEFAULT 0,
			net_roi DECIMAL(10,2) DEFAULT 0,
			net_amount DECIMAL(14,2) DEFAULT 0,
			net_order_count INT DEFAULT 0,
			net_settle_rate DECIMAL(8,4) DEFAULT 0,
			refund_1h_rate DECIMAL(8,4) DEFAULT 0,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			KEY idx_dist_material_date (stat_date),
			KEY idx_dist_material_account (account_name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音分销推素材'`,

		`CREATE TABLE IF NOT EXISTS op_douyin_dist_promote_hourly (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			stat_date DATE NOT NULL,
			account_name VARCHAR(200) NOT NULL,
			stat_hour VARCHAR(20) NOT NULL COMMENT '小时(如23:00)',
			cost DECIMAL(14,2) DEFAULT 0 COMMENT '消耗',
			settle_amount DECIMAL(14,2) DEFAULT 0 COMMENT '净成交金额',
			settle_count INT DEFAULT 0 COMMENT '净成交订单数',
			roi DECIMAL(10,2) DEFAULT 0,
			refund_rate DECIMAL(8,4) DEFAULT 0 COMMENT '退款率',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_dist_promote (stat_date, account_name, stat_hour),
			KEY idx_dist_promote_date (stat_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='抖音分销随心推(按小时)'`,
	}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}

	// 增量加列（忽略"列已存在"错误 MySQL 1060）
	addCols := []string{
		`ALTER TABLE users ADD COLUMN must_change_password TINYINT(1) NOT NULL DEFAULT 0 COMMENT '首次登录须改密码'`,
		`ALTER TABLE users ADD COLUMN department VARCHAR(100) DEFAULT '' COMMENT '所属部门'`,
		`ALTER TABLE users ADD COLUMN employee_id VARCHAR(50) DEFAULT '' COMMENT '工号'`,
		`ALTER TABLE users ADD COLUMN dingtalk_userid VARCHAR(64) DEFAULT '' COMMENT '钉钉用户ID'`,
		`ALTER TABLE users ADD COLUMN remark TEXT COMMENT '注册备注(权限申请说明)'`,
	}
	for _, alter := range addCols {
		if _, err := db.Exec(alter); err != nil && !strings.Contains(err.Error(), "1060") {
			return err
		}
	}

	if err := seedPermissions(db); err != nil {
		return err
	}
	if err := seedRoles(db); err != nil {
		return err
	}
	if err := seedRoleDefaultPermissions(db); err != nil {
		return err
	}
	if err := seedSuperAdminRolePermissions(db); err != nil {
		return err
	}
	if err := ensureDefaultAdmin(db); err != nil {
		return err
	}

	return nil
}

func seedPermissions(db *sql.DB) error {
	for _, seed := range permissionSeeds {
		if _, err := db.Exec(
			`INSERT INTO permissions (code, name, type) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE name = VALUES(name), type = VALUES(type)`,
			seed.Code, seed.Name, seed.Type,
		); err != nil {
			return err
		}
	}
	return nil
}

func seedRoles(db *sql.DB) error {
	for _, seed := range roleSeeds {
		if _, err := db.Exec(
			`INSERT INTO roles (code, name, description) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE name = VALUES(name), description = VALUES(description)`,
			seed.Code, seed.Name, seed.Description,
		); err != nil {
			return err
		}
	}
	return nil
}

func seedSuperAdminRolePermissions(db *sql.DB) error {
	var roleID int64
	if err := db.QueryRow(`SELECT id FROM roles WHERE code = 'super_admin'`).Scan(&roleID); err != nil {
		return err
	}

	for _, code := range allPermissionCodes() {
		var permissionID int64
		if err := db.QueryRow(`SELECT id FROM permissions WHERE code = ?`, code).Scan(&permissionID); err != nil {
			return err
		}
		if _, err := db.Exec(
			`INSERT IGNORE INTO role_permissions (role_id, permission_id) VALUES (?, ?)`,
			roleID, permissionID,
		); err != nil {
			return err
		}
	}
	return nil
}

func seedRoleDefaultPermissions(db *sql.DB) error {
	for roleCode, permissionCodes := range roleDefaultPermissions {
		var roleID int64
		if err := db.QueryRow(`SELECT id FROM roles WHERE code = ?`, roleCode).Scan(&roleID); err != nil {
			return err
		}
		for _, code := range permissionCodes {
			var permissionID int64
			if err := db.QueryRow(`SELECT id FROM permissions WHERE code = ?`, code).Scan(&permissionID); err != nil {
				return err
			}
			if _, err := db.Exec(
				`INSERT IGNORE INTO role_permissions (role_id, permission_id) VALUES (?, ?)`,
				roleID, permissionID,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureDefaultAdmin(db *sql.DB) error {
	var userCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		return err
	}
	if userCount > 0 {
		return nil
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(defaultAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	result, err := db.Exec(
		`INSERT INTO users (username, password_hash, real_name, status, must_change_password) VALUES (?, ?, ?, 'active', 1)`,
		defaultAdminUsername, string(passwordHash), "系统管理员",
	)
	if err != nil {
		return err
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return err
	}

	var roleID int64
	if err := db.QueryRow(`SELECT id FROM roles WHERE code = 'super_admin'`).Scan(&roleID); err != nil {
		return err
	}

	if _, err := db.Exec(`INSERT IGNORE INTO user_roles (user_id, role_id) VALUES (?, ?)`, userID, roleID); err != nil {
		return err
	}

	log.Printf("default admin account created username=%s", defaultAdminUsername)
	return nil
}

func (h *DashboardHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ip := clientIP(r)
	if locked, remaining := checkLoginLock(ip); locked {
		writeError(w, http.StatusTooManyRequests, fmt.Sprintf("登录失败次数过多，请 %d 分钟后重试", remaining))
		return
	}

	var req struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		Remember      bool   `json:"remember"`
		CaptchaID     string `json:"captchaId"`
		CaptchaAnswer int    `json:"captchaAnswer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "请输入账号和密码")
		return
	}

	if req.CaptchaID == "" {
		writeError(w, http.StatusBadRequest, "请完成验证码")
		return
	}
	if !verifyCaptcha(req.CaptchaID, req.CaptchaAnswer) {
		writeError(w, http.StatusBadRequest, "验证码错误或已过期")
		return
	}

	var (
		userID       int64
		passwordHash string
		realName     sql.NullString
		status       string
	)
	err := h.DB.QueryRow(
		`SELECT id, password_hash, real_name, status FROM users WHERE username = ?`,
		req.Username,
	).Scan(&userID, &passwordHash, &realName, &status)
	if errors.Is(err, sql.ErrNoRows) {
		remaining := recordLoginFailure(ip)
		if remaining > 0 {
			writeError(w, http.StatusUnauthorized, fmt.Sprintf("账号或密码错误，还剩 %d 次机会", remaining))
		} else {
			writeError(w, http.StatusTooManyRequests, fmt.Sprintf("登录失败次数过多，请 %d 分钟后重试", int(lockDuration.Minutes())))
		}
		return
	}
	if writeDatabaseError(w, err) {
		return
	}
	if status != "active" {
		writeError(w, http.StatusForbidden, "账号已被禁用，请联系管理员")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		remaining := recordLoginFailure(ip)
		if remaining > 0 {
			writeError(w, http.StatusUnauthorized, fmt.Sprintf("账号或密码错误，还剩 %d 次机会", remaining))
		} else {
			writeError(w, http.StatusTooManyRequests, fmt.Sprintf("登录失败次数过多，请 %d 分钟后重试", int(lockDuration.Minutes())))
		}
		return
	}

	clearLoginFailure(ip)

	token, err := generateSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建会话失败")
		return
	}
	tokenHash := hashSessionToken(token)
	duration := sessionDuration
	if !req.Remember {
		duration = shortSessionDuration
	}
	expiresAt := time.Now().Add(duration)

	if _, err := h.DB.Exec(
		`INSERT INTO user_sessions (user_id, token_hash, expires_at, ip, user_agent) VALUES (?, ?, ?, ?, ?)`,
		userID, tokenHash, expiresAt, ip, truncateString(r.UserAgent(), 255),
	); writeDatabaseError(w, err) {
		return
	}

	if _, err := h.DB.Exec(`UPDATE users SET last_login_at = NOW() WHERE id = ?`, userID); err != nil {
		log.Printf("update last_login_at failed: %v", err)
	}

	setSessionCookie(w, token, expiresAt, isSecureRequest(r))
	payload, err := h.loadAuthPayload(userID)
	if writeDatabaseError(w, err) {
		return
	}

	writeJSON(w, payload)
}

func (h *DashboardHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		if _, execErr := h.DB.Exec(`DELETE FROM user_sessions WHERE token_hash = ?`, hashSessionToken(cookie.Value)); execErr != nil {
			log.Printf("delete session failed: %v", execErr)
		}
	}

	clearSessionCookie(w)
	writeJSON(w, map[string]string{"message": "ok"})
}

func (h *DashboardHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if err := validatePassword(req.NewPassword, payload.User.Username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var currentHash string
	err := h.DB.QueryRow("SELECT IFNULL(password_hash,'') FROM users WHERE id=?", payload.User.ID).Scan(&currentHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询用户失败")
		return
	}

	if currentHash != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.OldPassword)); err != nil {
			writeError(w, http.StatusBadRequest, "当前密码错误")
			return
		}
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码加密失败")
		return
	}

	if _, err := h.DB.Exec("UPDATE users SET password_hash=?, must_change_password=0 WHERE id=?", string(newHash), payload.User.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "更新密码失败")
		return
	}
	if err := h.revokeUserSessions(payload.User.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "会话失效失败")
		return
	}
	clearSessionCookie(w)
	writeJSON(w, map[string]string{"message": "密码修改成功，请重新登录"})
}

func validatePassword(password, username string) error {
	if len(password) < 8 {
		return fmt.Errorf("密码至少8位")
	}
	if strings.EqualFold(password, username) {
		return fmt.Errorf("密码不能和用户名相同")
	}
	hasUpper, hasLower, hasDigit := false, false, false
	for _, c := range password {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return fmt.Errorf("密码必须包含大写字母、小写字母和数字")
	}
	return nil
}

func (h *DashboardHandler) Me(w http.ResponseWriter, r *http.Request) {
	payload, ok := authPayloadFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, payload)
}

func (h *DashboardHandler) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload, err := h.authPayloadFromRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), currentAuthPayloadKey, payload)
		next(w, r.WithContext(ctx))
	}
}

func (h *DashboardHandler) RequirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return h.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		payload, ok := authPayloadFromContext(r)
		if !ok || !hasPermission(payload, permission) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r)
	})
}

func (h *DashboardHandler) RequireAnyPermission(next http.HandlerFunc, permissions ...string) http.HandlerFunc {
	return h.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		payload, ok := authPayloadFromContext(r)
		if !ok {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		for _, permission := range permissions {
			if hasPermission(payload, permission) {
				next(w, r)
				return
			}
		}
		writeError(w, http.StatusForbidden, "forbidden")
	})
}

func authPayloadFromContext(r *http.Request) (*authPayload, bool) {
	payload, ok := r.Context().Value(currentAuthPayloadKey).(*authPayload)
	return payload, ok
}

func (h *DashboardHandler) authPayloadFromRequest(r *http.Request) (*authPayload, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, errors.New("missing session cookie")
	}

	tokenHash := hashSessionToken(cookie.Value)

	var userID int64
	var lastActiveAt time.Time
	err = h.DB.QueryRow(
		`SELECT user_id, IFNULL(last_active_at, created_at) FROM user_sessions WHERE token_hash = ? AND expires_at > NOW()`,
		tokenHash,
	).Scan(&userID, &lastActiveAt)
	if err != nil {
		return nil, err
	}

	payload, err := h.loadAuthPayload(userID)
	if err != nil {
		return nil, err
	}

	if !payload.IsSuperAdmin && time.Since(lastActiveAt) > idleTimeout {
		h.DB.Exec(`DELETE FROM user_sessions WHERE token_hash = ?`, tokenHash)
		return nil, errors.New("session idle timeout")
	}

	if _, execErr := h.DB.Exec(
		`UPDATE user_sessions SET last_active_at = NOW() WHERE token_hash = ?`,
		tokenHash,
	); execErr != nil {
		log.Printf("update session activity failed: %v", execErr)
	}

	return payload, nil
}

func (h *DashboardHandler) loadAuthPayload(userID int64) (*authPayload, error) {
	payload := &authPayload{
		DataScopes: authDataScopes{
			Depts:      []string{},
			Platforms:  []string{},
			Shops:      []string{},
			Warehouses: []string{},
			Domains:    []string{},
		},
	}

	var realName sql.NullString
	if err := h.DB.QueryRow(
		`SELECT id, username, real_name, must_change_password FROM users WHERE id = ? AND status = 'active'`,
		userID,
	).Scan(&payload.User.ID, &payload.User.Username, &realName, &payload.MustChangePassword); err != nil {
		return nil, err
	}
	payload.User.RealName = realName.String

	roleIDs := []int64{}
	roleRows, err := h.DB.Query(
		`SELECT r.id, r.code
		 FROM roles r
		 INNER JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer roleRows.Close()

	for roleRows.Next() {
		var roleID int64
		var roleCode string
		if err := roleRows.Scan(&roleID, &roleCode); err != nil {
			return nil, err
		}
		roleIDs = append(roleIDs, roleID)
		payload.Roles = append(payload.Roles, roleCode)
	}
	if err := roleRows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(payload.Roles)

	payload.IsSuperAdmin = containsString(payload.Roles, "super_admin")
	if payload.IsSuperAdmin {
		payload.Permissions = allPermissionCodes()
		sort.Strings(payload.Permissions)
		return payload, nil
	}

	permissionRows, err := h.DB.Query(
		`SELECT DISTINCT p.code
		 FROM permissions p
		 INNER JOIN role_permissions rp ON rp.permission_id = p.id
		 INNER JOIN user_roles ur ON ur.role_id = rp.role_id
		 WHERE ur.user_id = ?
		 ORDER BY p.code`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer permissionRows.Close()

	for permissionRows.Next() {
		var code string
		if err := permissionRows.Scan(&code); err != nil {
			return nil, err
		}
		payload.Permissions = append(payload.Permissions, code)
	}
	if err := permissionRows.Err(); err != nil {
		return nil, err
	}

	scopes, err := h.loadDataScopes(userID, roleIDs)
	if err != nil {
		return nil, err
	}
	payload.DataScopes = scopes

	return payload, nil
}

func (h *DashboardHandler) loadDataScopes(userID int64, roleIDs []int64) (authDataScopes, error) {
	scopes := authDataScopes{}
	query := `SELECT scope_type, scope_value FROM data_scopes WHERE (subject_type = 'user' AND subject_id = ?)`
	args := []interface{}{userID}

	if len(roleIDs) > 0 {
		placeholders := make([]string, 0, len(roleIDs))
		for _, roleID := range roleIDs {
			placeholders = append(placeholders, "?")
			args = append(args, roleID)
		}
		query += fmt.Sprintf(" OR (subject_type = 'role' AND subject_id IN (%s))", strings.Join(placeholders, ","))
	}
	query += " ORDER BY scope_type, scope_value"

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		return scopes, err
	}
	defer rows.Close()

	for rows.Next() {
		var scopeType string
		var scopeValue string
		if err := rows.Scan(&scopeType, &scopeValue); err != nil {
			return scopes, err
		}
		switch scopeType {
		case "dept":
			scopes.Depts = append(scopes.Depts, scopeValue)
		case "platform":
			scopes.Platforms = append(scopes.Platforms, scopeValue)
		case "shop":
			scopes.Shops = append(scopes.Shops, scopeValue)
		case "warehouse":
			scopes.Warehouses = append(scopes.Warehouses, scopeValue)
		case "domain":
			scopes.Domains = append(scopes.Domains, scopeValue)
		}
	}
	if err := rows.Err(); err != nil {
		return scopes, err
	}

	scopes.Depts = uniqueSortedStrings(scopes.Depts)
	scopes.Platforms = uniqueSortedStrings(scopes.Platforms)
	scopes.Shops = uniqueSortedStrings(scopes.Shops)
	scopes.Warehouses = uniqueSortedStrings(scopes.Warehouses)
	scopes.Domains = uniqueSortedStrings(scopes.Domains)

	return scopes, nil
}

func generateSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasPermission(payload *authPayload, permission string) bool {
	if payload == nil {
		return false
	}
	if payload.IsSuperAdmin || containsString(payload.Roles, "super_admin") {
		return true
	}
	if permission == "" {
		return false
	}
	return containsString(payload.Permissions, permission)
}

func isSecureRequest(r *http.Request) bool {
	if r != nil && r.TLS != nil {
		return true
	}
	if r == nil {
		return false
	}
	if !isTrustedProxyRemoteAddr(r.RemoteAddr) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func isTrustedProxyRemoteAddr(remoteAddr string) bool {
	host := strings.TrimSpace(remoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	if isTrustedProxyRemoteAddr(r.RemoteAddr) {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			return truncateString(forwarded, 64)
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return truncateString(realIP, 64)
		}
	}

	host := strings.TrimSpace(r.RemoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return truncateString(host, 64)
}

func (h *DashboardHandler) revokeUserSessions(userID int64) error {
	if h == nil || h.DB == nil {
		return errors.New("db is not initialized")
	}
	_, err := h.DB.Exec("DELETE FROM user_sessions WHERE user_id = ?", userID)
	return err
}

func truncateString(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	runes := []rune(value)
	for len(string(runes)) > maxLen {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}

// ==================== 钉钉 OAuth ====================

func (h *DashboardHandler) DingtalkAuthURL(w http.ResponseWriter, r *http.Request) {
	if h.DingClientID == "" {
		writeError(w, http.StatusBadRequest, "钉钉登录未配置")
		return
	}
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "http://192.168.200.48:3000"
	}
	parsed, _ := url.Parse(referer)
	redirectURI := fmt.Sprintf("%s://%s/dingtalk/callback", parsed.Scheme, parsed.Host)

	state := r.URL.Query().Get("state")
	if state != "login" && state != "bind" {
		state = "login"
	}
	authURL := fmt.Sprintf(
		"https://login.dingtalk.com/oauth2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=openid+corpid&prompt=consent&state=%s",
		h.DingClientID, url.QueryEscape(redirectURI), url.QueryEscape(state),
	)
	writeJSON(w, map[string]string{"url": authURL})
}

func (h *DashboardHandler) DingtalkLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Code        string `json:"code"`
		Remark      string `json:"remark"`
		PendingToken string `json:"pendingToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.PendingToken != "" {
		val, ok := dingtalkPendingUsers.LoadAndDelete(req.PendingToken)
		if !ok {
			writeError(w, http.StatusBadRequest, "注册信息已过期，请重新扫码")
			return
		}
		pending := val.(*dingtalkPendingUser)
		if time.Now().After(pending.Expires) {
			writeError(w, http.StatusBadRequest, "注册信息已过期，请重新扫码")
			return
		}
		remark := strings.TrimSpace(req.Remark)
		if pending.Department != "" {
			remark = "【" + pending.Department + "】" + remark
		}
		dtID := pending.UnionId
		if dtID == "" {
			dtID = pending.OpenId
		}
		mobile := strings.TrimSpace(pending.Mobile)
		username := mobile
		if username == "" {
			username = "dt_" + dtID[:12]
		}
		result, err := h.DB.Exec(
			`INSERT INTO users (username, password_hash, real_name, phone, dingtalk_userid, status, remark) VALUES (?, '', ?, ?, ?, 'pending', ?)`,
			username, pending.Nick, mobile, dtID, remark,
		)
		if err != nil {
			writeError(w, http.StatusConflict, "该手机号已注册，请联系管理员")
			return
		}
		newID, _ := result.LastInsertId()
		log.Printf("dingtalk new user registered: id=%d name=%s phone=%s", newID, pending.Nick, mobile)
		writeJSON(w, map[string]interface{}{"pending": true, "message": "注册申请已提交，请等待管理员审批开通"})
		return
	}

	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	// 1. 用 code 换 access_token
	tokenBody, _ := json.Marshal(map[string]string{
		"clientId":     h.DingClientID,
		"clientSecret": h.DingClientSecret,
		"code":         req.Code,
		"grantType":    "authorization_code",
	})
	tokenResp, err := http.Post("https://api.dingtalk.com/v1.0/oauth2/userAccessToken", "application/json", bytes.NewReader(tokenBody))
	if err != nil {
		writeError(w, http.StatusBadGateway, "钉钉服务器连接失败")
		return
	}
	defer tokenResp.Body.Close()
	tokenRespBody, _ := io.ReadAll(tokenResp.Body)

	var tokenData struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(tokenRespBody, &tokenData)
	if tokenData.AccessToken == "" {
		log.Printf("dingtalk token error: %s", string(tokenRespBody))
		writeError(w, http.StatusBadRequest, "钉钉授权失败，请重试")
		return
	}

	// 2. 用 access_token 获取用户信息
	meReq, _ := http.NewRequest("GET", "https://api.dingtalk.com/v1.0/contact/users/me", nil)
	meReq.Header.Set("x-acs-dingtalk-access-token", tokenData.AccessToken)
	meResp, err := http.DefaultClient.Do(meReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "获取钉钉用户信息失败")
		return
	}
	defer meResp.Body.Close()
	meRespBody, _ := io.ReadAll(meResp.Body)

	var dtUser struct {
		Nick    string `json:"nick"`
		UnionId string `json:"unionId"`
		OpenId  string `json:"openId"`
		Mobile  string `json:"mobile"`
	}
	json.Unmarshal(meRespBody, &dtUser)
	if dtUser.UnionId == "" && dtUser.OpenId == "" {
		log.Printf("dingtalk user error: %s", string(meRespBody))
		writeError(w, http.StatusBadRequest, "获取钉钉身份失败")
		return
	}
	dtID := dtUser.UnionId
	if dtID == "" {
		dtID = dtUser.OpenId
	}

	// 3. 查 dingtalk_userid 是否已绑定
	var userID int64
	var userStatus string
	err = h.DB.QueryRow(
		`SELECT id, status FROM users WHERE dingtalk_userid = ? AND dingtalk_userid != ''`, dtID,
	).Scan(&userID, &userStatus)

	if err == nil {
		// 已绑定
		if userStatus == "pending" {
			remark := strings.TrimSpace(req.Remark)
			if remark != "" {
				h.DB.Exec(`UPDATE users SET remark = ? WHERE id = ?`, remark, userID)
			}
			writeJSON(w, map[string]interface{}{"pending": true, "message": "注册申请已提交，请等待管理员审批开通"})
			return
		}
		if userStatus != "active" {
			writeError(w, http.StatusForbidden, "账号已被禁用")
			return
		}
		h.createSessionAndRespond(w, r, userID)
		return
	}

	// 4. 用手机号匹配已有账号
	mobile := strings.TrimSpace(dtUser.Mobile)
	if mobile != "" {
		err = h.DB.QueryRow(
			`SELECT id, status FROM users WHERE (phone = ? OR username = ?) AND dingtalk_userid = ''`, mobile, mobile,
		).Scan(&userID, &userStatus)
		if err == nil {
			// 匹配到 → 自动绑定
			h.DB.Exec(`UPDATE users SET dingtalk_userid = ? WHERE id = ?`, dtID, userID)
			if userStatus != "active" {
				writeError(w, http.StatusForbidden, "账号已被禁用")
				return
			}
			h.createSessionAndRespond(w, r, userID)
			return
		}
	}

	// 5. 都没找到 → 查部门 + 缓存信息等待填写备注
	dept := h.getDingtalkDepartment(dtUser.UnionId)
	pendingToken, _ := generateSessionToken()
	dingtalkPendingUsers.Store(pendingToken, &dingtalkPendingUser{
		Nick:       dtUser.Nick,
		UnionId:    dtUser.UnionId,
		OpenId:     dtUser.OpenId,
		Mobile:     dtUser.Mobile,
		Department: dept,
		Expires:    time.Now().Add(10 * time.Minute),
	})
	writeJSON(w, map[string]interface{}{"needRemark": true, "nick": dtUser.Nick, "department": dept, "pendingToken": pendingToken})
	return
}

func (h *DashboardHandler) getDingtalkDepartment(unionId string) string {
	// 1. 获取企业 access_token
	tokenBody, _ := json.Marshal(map[string]string{
		"appKey":    h.DingClientID,
		"appSecret": h.DingClientSecret,
	})
	tokenResp, err := http.Post("https://api.dingtalk.com/v1.0/oauth2/accessToken", "application/json", bytes.NewReader(tokenBody))
	if err != nil {
		log.Printf("dingtalk corp token error: %v", err)
		return ""
	}
	defer tokenResp.Body.Close()
	tokenRespBody, _ := io.ReadAll(tokenResp.Body)
	var corpToken struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(tokenRespBody, &corpToken)
	if corpToken.AccessToken == "" {
		log.Printf("dingtalk corp token empty: %s", string(tokenRespBody))
		return ""
	}

	// 2. 通过 unionId 查 userid
	unionBody, _ := json.Marshal(map[string]string{"unionid": unionId})
	unionReq, _ := http.NewRequest("POST", "https://oapi.dingtalk.com/topapi/user/getbyunionid?access_token="+corpToken.AccessToken, bytes.NewReader(unionBody))
	unionReq.Header.Set("Content-Type", "application/json")
	unionResp, err := http.DefaultClient.Do(unionReq)
	if err != nil {
		log.Printf("dingtalk getbyunionid error: %v", err)
		return ""
	}
	defer unionResp.Body.Close()
	unionRespBody, _ := io.ReadAll(unionResp.Body)
	var unionData struct {
		ErrCode int `json:"errcode"`
		Result  struct {
			UserId string `json:"userid"`
		} `json:"result"`
	}
	json.Unmarshal(unionRespBody, &unionData)
	if unionData.Result.UserId == "" {
		log.Printf("dingtalk getbyunionid empty: %s", string(unionRespBody))
		return ""
	}

	// 3. 查用户详情获取部门ID
	detailBody, _ := json.Marshal(map[string]string{"userid": unionData.Result.UserId})
	detailReq, _ := http.NewRequest("POST", "https://oapi.dingtalk.com/topapi/v2/user/get?access_token="+corpToken.AccessToken, bytes.NewReader(detailBody))
	detailReq.Header.Set("Content-Type", "application/json")
	detailResp, err := http.DefaultClient.Do(detailReq)
	if err != nil {
		log.Printf("dingtalk user detail error: %v", err)
		return ""
	}
	defer detailResp.Body.Close()
	detailRespBody, _ := io.ReadAll(detailResp.Body)
	var detailData struct {
		ErrCode int `json:"errcode"`
		Result  struct {
			DeptIdList []int64 `json:"dept_id_list"`
		} `json:"result"`
	}
	json.Unmarshal(detailRespBody, &detailData)
	if len(detailData.Result.DeptIdList) == 0 {
		log.Printf("dingtalk user detail no dept: %s", string(detailRespBody))
		return ""
	}

	// 4. 查部门名称
	deptNames := []string{}
	for _, deptId := range detailData.Result.DeptIdList {
		deptBody, _ := json.Marshal(map[string]interface{}{"dept_id": deptId})
		deptReq, _ := http.NewRequest("POST", "https://oapi.dingtalk.com/topapi/v2/department/get?access_token="+corpToken.AccessToken, bytes.NewReader(deptBody))
		deptReq.Header.Set("Content-Type", "application/json")
		deptResp, err := http.DefaultClient.Do(deptReq)
		if err != nil {
			continue
		}
		deptRespBody, _ := io.ReadAll(deptResp.Body)
		deptResp.Body.Close()
		var deptData struct {
			ErrCode int `json:"errcode"`
			Result  struct {
				Name string `json:"name"`
			} `json:"result"`
		}
		json.Unmarshal(deptRespBody, &deptData)
		if deptData.Result.Name != "" {
			deptNames = append(deptNames, deptData.Result.Name)
		}
	}

	result := strings.Join(deptNames, " / ")
	log.Printf("dingtalk dept for %s: %s", unionId, result)
	return result
}

func (h *DashboardHandler) createSessionAndRespond(w http.ResponseWriter, r *http.Request, userID int64) {
	token, err := generateSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成会话失败")
		return
	}
	tokenHash := hashSessionToken(token)
	expiresAt := time.Now().Add(sessionDuration)
	ip := clientIP(r)

	if _, err := h.DB.Exec(
		`INSERT INTO user_sessions (user_id, token_hash, expires_at, ip, user_agent) VALUES (?, ?, ?, ?, ?)`,
		userID, tokenHash, expiresAt, ip, truncateString(r.UserAgent(), 255),
	); err != nil {
		writeError(w, http.StatusInternalServerError, "创建会话失败")
		return
	}
	h.DB.Exec(`UPDATE users SET last_login_at = NOW() WHERE id = ?`, userID)
	setSessionCookie(w, token, expiresAt, isSecureRequest(r))

	payload, err := h.loadAuthPayload(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "加载用户信息失败")
		return
	}
	writeJSON(w, payload)
}

func (h *DashboardHandler) DingtalkBind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Code   string `json:"code"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	// 解绑
	if req.Action == "unbind" {
		h.DB.Exec(`UPDATE users SET dingtalk_userid = '' WHERE id = ?`, payload.User.ID)
		writeJSON(w, map[string]string{"message": "已解绑钉钉"})
		return
	}

	// 绑定：用 code 换钉钉信息
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	tokenBody, _ := json.Marshal(map[string]string{
		"clientId":     h.DingClientID,
		"clientSecret": h.DingClientSecret,
		"code":         req.Code,
		"grantType":    "authorization_code",
	})
	tokenResp, err := http.Post("https://api.dingtalk.com/v1.0/oauth2/userAccessToken", "application/json", bytes.NewReader(tokenBody))
	if err != nil {
		writeError(w, http.StatusBadGateway, "钉钉服务器连接失败")
		return
	}
	defer tokenResp.Body.Close()
	tokenRespBody, _ := io.ReadAll(tokenResp.Body)

	var tokenData struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(tokenRespBody, &tokenData)
	if tokenData.AccessToken == "" {
		writeError(w, http.StatusBadRequest, "钉钉授权失败")
		return
	}

	meReq, _ := http.NewRequest("GET", "https://api.dingtalk.com/v1.0/contact/users/me", nil)
	meReq.Header.Set("x-acs-dingtalk-access-token", tokenData.AccessToken)
	meResp, err := http.DefaultClient.Do(meReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "获取钉钉用户信息失败")
		return
	}
	defer meResp.Body.Close()
	meBody, _ := io.ReadAll(meResp.Body)

	var dtUser struct {
		Nick    string `json:"nick"`
		UnionId string `json:"unionId"`
		OpenId  string `json:"openId"`
		Mobile  string `json:"mobile"`
	}
	json.Unmarshal(meBody, &dtUser)
	dtID := dtUser.UnionId
	if dtID == "" {
		dtID = dtUser.OpenId
	}
	if dtID == "" {
		writeError(w, http.StatusBadRequest, "获取钉钉身份失败")
		return
	}

	// 检查是否已被其他账号绑定
	var existID int64
	err = h.DB.QueryRow(`SELECT id FROM users WHERE dingtalk_userid = ? AND id != ?`, dtID, payload.User.ID).Scan(&existID)
	if err == nil {
		writeError(w, http.StatusConflict, "该钉钉账号已绑定其他用户")
		return
	}

	sets := []string{"dingtalk_userid = ?"}
	args := []interface{}{dtID}
	if strings.TrimSpace(dtUser.Nick) != "" {
		sets = append(sets, "real_name = ?")
		args = append(args, strings.TrimSpace(dtUser.Nick))
	}
	if strings.TrimSpace(dtUser.Mobile) != "" {
		sets = append(sets, "phone = ?")
		args = append(args, strings.TrimSpace(dtUser.Mobile))
	}
	args = append(args, payload.User.ID)
	h.DB.Exec(`UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	writeJSON(w, map[string]string{"message": "钉钉绑定成功", "nick": dtUser.Nick, "mobile": dtUser.Mobile})
}

func (h *DashboardHandler) StartCleanupRoutines() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		now := time.Now()

		captchaMu.Lock()
		for id, entry := range captchaStore {
			if now.After(entry.expiresAt) {
				delete(captchaStore, id)
			}
		}
		captchaMu.Unlock()

		loginMu.Lock()
		for ip, attempt := range loginAttempts {
			if !attempt.lockedAt.IsZero() && now.Sub(attempt.lockedAt) > 30*time.Minute {
				delete(loginAttempts, ip)
			}
		}
		loginMu.Unlock()

		dingtalkPendingUsers.Range(func(key, value interface{}) bool {
			if entry, ok := value.(*dingtalkPendingUser); ok {
				if now.After(entry.Expires) {
					dingtalkPendingUsers.Delete(key)
				}
			}
			return true
		})

		h.DB.Exec("DELETE FROM user_sessions WHERE expires_at < ?", now)
	}
}
