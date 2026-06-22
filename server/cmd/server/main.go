package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"bi-dashboard/internal/ai_assistant"
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/dingtalk"
	"bi-dashboard/internal/handler"
	"bi-dashboard/internal/yingdao"
	"bi-dashboard/internal/yonsuite"

	"github.com/getsentry/sentry-go"
	_ "github.com/go-sql-driver/mysql"
)

// initStructuredLogging 初始化 slog (JSON 输出) 并接管 log 包默认 logger
//   - LOG_FORMAT=json (默认) → JSON 单行 / 日志聚合工具 (ELK/Loki) 可直接消费
//   - LOG_FORMAT=text         → 人类可读, 本地开发用
//   - LOG_LEVEL=debug|info|warn|error (默认 info)
func initStructuredLogging() {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if strings.ToLower(os.Getenv("LOG_FORMAT")) == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// 接管 log 包默认 output: 让 log.Printf/Println/Fatalf 也走 slog handler
	// 副作用: 所有现存 88 处 log.* 调用自动变 JSON 行 (msg 字段含原文本)
	log.SetFlags(0) // 关掉默认 timestamp, slog 自带
	log.SetOutput(stdLoggerWriter{})
}

// stdLoggerWriter 把 log 包写入桥接到 slog
type stdLoggerWriter struct{}

func (stdLoggerWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	slog.Info(msg)
	return len(p), nil
}

// statusRecorder 拦 ResponseWriter 记录状态码 (访问日志用)
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap 暴露底层 ResponseWriter, 让 http.NewResponseController 能穿透本包装层
// 拿到真正的连接 (否则 SetWriteDeadline/Flush/Hijack 全部 ErrNotSupported)。
// 用友批量出库/转换接口靠它清掉 120s WriteTimeout, 大批量才不会被半路掐断 → 防重复提交。
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func main() {
	initStructuredLogging()

	// Sentry 错误监控 (DSN 留空时静默禁用, 不影响功能)
	// 配置: 环境变量 SENTRY_DSN=https://xxx@sentry.io/12345 + SENTRY_ENV=production
	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		environment := os.Getenv("SENTRY_ENV")
		if environment == "" {
			environment = "production"
		}
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              dsn,
			Environment:      environment,
			Release:          os.Getenv("SENTRY_RELEASE"), // 空字符串 → SDK 不带 release
			TracesSampleRate: 0.0,                          // 仅捕错误, 不做性能追踪 (省 quota)
			AttachStacktrace: true,
		}); err != nil {
			log.Printf("[sentry] init failed: %v (继续启动, 不阻塞服务)", err)
		} else {
			log.Printf("[sentry] enabled, environment=%s", environment)
			defer sentry.Flush(2 * time.Second)
		}
	} else {
		log.Println("[sentry] disabled (未配置 SENTRY_DSN)")
	}

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// 关键凭证必填校验：避免部署遗漏导致 webhook 无校验全开放
	if cfg.Webhook.Secret == "" {
		log.Fatalf("config error: webhook.secret 必须配置（防止 webhook 接口被未授权调用）")
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(40)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	log.Println("Database connected")

	if err := handler.EnsureAuthSchemaAndSeed(db); err != nil {
		log.Fatalf("init auth schema: %v", err)
	}
	log.Println("Auth schema ready")

	notifier := dingtalk.NewNotifier(cfg.DingTalk.NotifyAppKey, cfg.DingTalk.NotifyAppSecret, cfg.DingTalk.NotifyRobotCode)
	if notifier != nil {
		log.Println("DingTalk notifier ready (反馈回复 + 定时任务失败将推送 admin)")
	} else {
		log.Println("DingTalk notifier disabled (未配置 notify_app_key/notify_app_secret)")
	}

	yingdaoClient := yingdao.NewClient(
		cfg.YingDao.AccessKeyID, cfg.YingDao.AccessKeySecret,
		cfg.YingDao.AuthURL, cfg.YingDao.BizURL, cfg.YingDao.DefaultAccount,
	)
	if yingdaoClient.Configured() {
		log.Printf("YingDao RPA client ready (account=%s)", cfg.YingDao.DefaultAccount)
	} else {
		log.Println("YingDao RPA client disabled (未配置 yingdao.access_key_id/access_key_secret)")
	}

	// v1.73.0 BI 智能助手: classify 用 primary (准), format 用 fast (快)
	var aiSvc *ai_assistant.Service
	if cfg.AIAssistant.Enabled && cfg.AIAssistant.LLMAPIKey != "" {
		primaryClient := ai_assistant.NewLLMClient(
			cfg.AIAssistant.LLMBaseURL,
			cfg.AIAssistant.LLMAPIKey,
			cfg.AIAssistant.LLMModelPrimary,
			cfg.AIAssistant.LLMTimeoutSecs,
		)
		var fastClient *ai_assistant.LLMClient
		if cfg.AIAssistant.LLMModelFallback != "" {
			fastClient = ai_assistant.NewLLMClient(
				cfg.AIAssistant.LLMBaseURL,
				cfg.AIAssistant.LLMAPIKey,
				cfg.AIAssistant.LLMModelFallback,
				cfg.AIAssistant.LLMTimeoutSecs,
			)
		}
		aiSvc = &ai_assistant.Service{
			DB:               db,
			Client:           primaryClient,
			ClientFast:       fastClient,
			CacheEnabled:     cfg.AIAssistant.CacheEnabled,
			CacheTTL:         time.Duration(cfg.AIAssistant.CacheTTLSeconds) * time.Second,
			WarmCacheEnabled: cfg.AIAssistant.WarmCacheEnabled,
			WarmCacheHour:    cfg.AIAssistant.WarmCacheHour,
			WarmCacheMinute:  cfg.AIAssistant.WarmCacheMinute,
		}
		log.Printf("AI Assistant ready (provider=%s primary=%s fast=%s cache=%v ttl=%ds warm=%v@%02d:%02d)",
			cfg.AIAssistant.LLMProvider, cfg.AIAssistant.LLMModelPrimary, cfg.AIAssistant.LLMModelFallback,
			cfg.AIAssistant.CacheEnabled, cfg.AIAssistant.CacheTTLSeconds,
			cfg.AIAssistant.WarmCacheEnabled, cfg.AIAssistant.WarmCacheHour, cfg.AIAssistant.WarmCacheMinute)
		// v1.74.0 P2: warm cache goroutine (跟 bi-server 进程同生命周期)
		go aiSvc.RunWarmCacheLoop(context.Background())
	} else {
		log.Println("AI Assistant disabled (config.ai_assistant.enabled=false 或未配 llm_api_key)")
	}

	// v1.75.7: 用友 YS 客户端 (查凭证明细, 仅 bi-server 主进程用)
	var ysClient *yonsuite.Client
	if cfg.YonSuite.AppKey != "" && cfg.YonSuite.AppSecret != "" && cfg.YonSuite.BaseURL != "" {
		ysClient = yonsuite.NewClient(cfg.YonSuite.AppKey, cfg.YonSuite.AppSecret, cfg.YonSuite.BaseURL)
		log.Printf("YS YonBIP client ready (base=%s)", cfg.YonSuite.BaseURL)
	} else {
		log.Println("YS YonBIP disabled (未配置 yonsuite.appkey/secret)")
	}

	h := &handler.DashboardHandler{
		DB:               db,
		DingToken:        cfg.DingTalk.WebhookToken,
		DingSecret:       cfg.DingTalk.WebhookSecret,
		DingClientID:     cfg.DingTalk.ClientID,
		DingClientSecret: cfg.DingTalk.ClientSecret,
		DingCallbackHost: cfg.DingTalk.CallbackHost,
		HesiAppKey:       cfg.Hesi.AppKey,
		HesiSecret:       cfg.Hesi.Secret,
		WebhookSecret:    cfg.Webhook.Secret,
		Notifier:         notifier,
		YingDao:          yingdaoClient,
		AIAssistant:      aiSvc,
		YS:               ysClient,
	}

	// 启动定时任务健康巡检 (失败/卡死自动钉钉告警 admin)
	go h.StartTaskHealthMonitor()

	// 启动影刀 RPA 状态巡检 (30s 扫 running 任务, 主动更新终态 + 钉钉通知)
	h.StartYingDaoStatusReaper()

	mux := http.NewServeMux()

	allowedOrigins := map[string]bool{
		"http://localhost:3000":        true,
		"http://127.0.0.1:3000":        true,
		"http://192.168.200.48:3000":   true,
		"http://songxianxian.local":    true,
		"http://bi.songxianxian.local": true,
	}

	// sentryRecover 包在 corsHandler 外层: 捕获 handler panic → 上报 Sentry → 返 500
	// 不会吞错: 仍 log + 给客户端 500, 只是额外把 panic 推到 Sentry 让 admin 收到告警
	sentryRecover := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					hub := sentry.CurrentHub().Clone()
					hub.Scope().SetTag("path", r.URL.Path)
					hub.Scope().SetTag("method", r.Method)
					hub.Recover(rec)
					hub.Flush(2 * time.Second)
					slog.Error("panic", "method", r.Method, "path", r.URL.Path, "error", fmt.Sprintf("%v", rec))
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next(w, r)
		}
	}

	// accessLog 记录每个 HTTP 请求 (method/path/status/duration_ms/client_ip)
	// 输出 JSON 行, 便于 ELK/Loki 等聚合: {"level":"INFO","msg":"http","method":"GET",...}
	accessLog := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			next(rec, r)
			elapsed := time.Since(start)
			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			} else if rec.status >= 400 {
				level = slog.LevelWarn
			}
			slog.LogAttrs(r.Context(), level, "http",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int64("duration_ms", elapsed.Milliseconds()),
				slog.String("remote", r.RemoteAddr),
			)
		}
	}

	corsHandler := func(next http.HandlerFunc) http.HandlerFunc {
		return accessLog(sentryRecover(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowedOrigins[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
			if r.Method == "OPTIONS" {
				w.WriteHeader(200)
				return
			}
			if r.Method == "POST" || r.Method == "PUT" {
				// 文件上传路径放大到 10MB (头像 5MB + 反馈附件 5×2MB), 其他路径 2MB 防 DoS
				if r.URL.Path == "/api/feedback" || r.URL.Path == "/api/user/avatar" {
					r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB
				} else {
					r.Body = http.MaxBytesReader(w, r.Body, 2<<20) // 2MB
				}
			}
			next(w, r)
		}))
	}

	protected := func(next http.HandlerFunc) http.HandlerFunc {
		return corsHandler(h.RequireAuth(next))
	}
	pageProtected := func(permission string, next http.HandlerFunc) http.HandlerFunc {
		return corsHandler(h.RequirePermission(permission, next))
	}
	pageAnyProtected := func(next http.HandlerFunc, permissions ...string) http.HandlerFunc {
		return corsHandler(h.RequireAnyPermission(next, permissions...))
	}
	pageAllProtected := func(next http.HandlerFunc, permissions ...string) http.HandlerFunc {
		return corsHandler(h.RequireAllPermissions(next, permissions...))
	}
	adminUsers := func(next http.HandlerFunc) http.HandlerFunc {
		return corsHandler(h.RequirePermission("user.manage", next))
	}
	adminRoles := func(next http.HandlerFunc) http.HandlerFunc {
		return corsHandler(h.RequirePermission("role.manage", next))
	}
	adminMeta := func(next http.HandlerFunc) http.HandlerFunc {
		return corsHandler(h.RequireAnyPermission(next, "user.manage", "role.manage"))
	}

	mux.HandleFunc("/api/auth/captcha", corsHandler(h.GetCaptcha))
	mux.HandleFunc("/api/auth/captcha/verify", corsHandler(h.VerifyCaptchaOnly))
	mux.HandleFunc("/api/auth/login", corsHandler(h.Login))
	mux.HandleFunc("/api/auth/logout", corsHandler(h.RequireAuth(h.Logout)))
	mux.HandleFunc("/api/auth/me", corsHandler(h.RequireAuth(h.Me)))
	mux.HandleFunc("/api/auth/change-password", corsHandler(h.RequireAuth(h.ChangePassword)))
	mux.HandleFunc("/api/auth/dingtalk/url", corsHandler(h.DingtalkAuthURL))
	mux.HandleFunc("/api/auth/dingtalk/login", corsHandler(h.DingtalkLogin))
	mux.HandleFunc("/api/user/dingtalk", corsHandler(h.RequireAuth(h.DingtalkBind)))
	mux.HandleFunc("/api/admin/meta", adminMeta(h.AdminMeta))
	mux.HandleFunc("/api/admin/users/batch", adminUsers(h.AdminUsersBatchImport))
	mux.HandleFunc("/api/admin/users", adminUsers(h.AdminUsers))
	mux.HandleFunc("/api/admin/users/", adminUsers(h.AdminUserByPath))
	mux.HandleFunc("/api/admin/online-users", adminUsers(h.AdminOnlineUsers))
	mux.HandleFunc("/api/admin/roles", adminRoles(h.AdminRoles))
	mux.HandleFunc("/api/admin/roles/", adminRoles(h.AdminRoleByPath))
	mux.HandleFunc("/api/audit/page-view", protected(h.AuditLogPageView))
	mux.HandleFunc("/api/admin/audit-logs", adminMeta(h.AdminAuditLogs))
	mux.HandleFunc("/api/admin/pending-counts", protected(h.AdminPendingCounts))
	// 用户操作活动: 个人中心「我的活动」(看自己) + 系统设置「全员活动」(管理员看所有人)
	mux.HandleFunc("/api/user/activity", protected(h.UserActivity))
	mux.HandleFunc("/api/admin/user-activity", adminUsers(h.AdminUserActivity))
	mux.HandleFunc("/api/admin/users-activity", adminUsers(h.AdminUsersActivity))

	// 用友 YonBIP 批量出库工具 (系统设置小工具, 单人高频用)。
	// 权限 system.yonbip:use; super_admin 自动通过, 其他人没分配则进不来。
	// export-execute 会真写用友 (建单+批次转换+审核, 不可逆)。
	mux.HandleFunc("/api/yonbip/export-plan", pageProtected("system.yonbip:use", h.YonbipExportPlan))
	mux.HandleFunc("/api/yonbip/export-execute", pageProtected("system.yonbip:use", h.YonbipExportExecute))
	// 批次转换 / 库存状态转换工具 (同权限)。convert-stock 只读查现存量; convert-execute 真写用友 (建转换单+审核, 不可逆)。
	mux.HandleFunc("/api/yonbip/convert-stock", pageProtected("system.yonbip:use", h.YonbipConvertStock))
	mux.HandleFunc("/api/yonbip/convert-execute", pageProtected("system.yonbip:use", h.YonbipConvertExecute))
	// convert-options 只读: 给货品/仓库可搜下拉提供选项 (取本地 ys_stock, 不调用友)。
	mux.HandleFunc("/api/yonbip/convert-options", pageProtected("system.yonbip:use", h.YonbipConvertOptions))
	// 新增采购订单 (同权限): po-preview 上传Excel翻译算价预览(不建单); po-commit 确认建单(真写用友+防重, 不可逆)。
	mux.HandleFunc("/api/yonbip/po-preview", pageProtected("system.yonbip:use", h.YonbipPOPreview))
	mux.HandleFunc("/api/yonbip/po-commit", pageProtected("system.yonbip:use", h.YonbipPOCommit))

	// T-1 数据看板：昨天/前天/上月数据永远不变，缓存 24 小时
	// 同步脚本完成后会调 ClearCacheByPrefix 主动清除（详见 supply_chain.go / stock.go）
	// 历史背景：曾命名 cache5m 但 TTL 为 60min，现统一为 cache24h（v0.56.7）
	cache24h := func(fn http.HandlerFunc) http.HandlerFunc { return h.WithCache(24*time.Hour, fn) }
	// v1.73.2: hesi 数据 sync-hesi schtasks 每 15min 一次, cache TTL 必须同频, 否则用户看 24h 旧数据
	cache15m := func(fn http.HandlerFunc) http.HandlerFunc { return h.WithCache(15*time.Minute, fn) }
	// 期货行情盘中实时: sync-futures-realtime 每 5min 更新快照并清此缓存, TTL 同频兜底
	cache5m := func(fn http.HandlerFunc) http.HandlerFunc { return h.WithCache(5*time.Minute, fn) }

	mux.HandleFunc("/api/overview", pageAnyProtected(cache24h(h.GetOverview),
		"overview:view", "finance.overview:view", "finance.monthly_profit:view",
	))
	mux.HandleFunc("/api/department", pageAnyProtected(cache24h(h.GetDepartmentDetail),
		"ecommerce.store_preview:view", "ecommerce.store_dashboard:view", "ecommerce.product_dashboard:view",
		"social.store_preview:view", "social.store_dashboard:view", "social.product_dashboard:view",
		"offline.store_preview:view", "offline.store_dashboard:view", "offline.product_dashboard:view",
		"distribution.store_preview:view", "distribution.store_dashboard:view", "distribution.product_dashboard:view",
		"instant_retail.store_preview:view", "instant_retail.store_dashboard:view", "instant_retail.product_dashboard:view",
		"finance.department_profit:view", "finance.monthly_profit:view", "finance.product_profit:view",
	))
	mux.HandleFunc("/api/tmall/ops", pageAnyProtected(cache24h(h.GetTmallOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
		"instant_retail.store_dashboard:view",
	))
	mux.HandleFunc("/api/vip/ops", pageAnyProtected(cache24h(h.GetVipOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
		"instant_retail.store_dashboard:view",
	))
	mux.HandleFunc("/api/pdd/ops", pageAnyProtected(cache24h(h.GetPddOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
		"instant_retail.store_dashboard:view",
	))
	mux.HandleFunc("/api/jd/ops", pageAnyProtected(cache24h(h.GetJdOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
		"instant_retail.store_dashboard:view",
	))
	mux.HandleFunc("/api/tmallcs/ops", pageAnyProtected(cache24h(h.GetTmallcsOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
		"instant_retail.store_dashboard:view",
	))
	mux.HandleFunc("/api/feigua", pageProtected("social.feigua:view", cache24h(h.GetFeiguaData)))
	mux.HandleFunc("/api/xiaohongshu/filters", pageProtected("social.xiaohongshu:view", cache24h(h.GetXhsFilters)))
	mux.HandleFunc("/api/xiaohongshu/note", pageProtected("social.xiaohongshu:view", cache24h(h.GetXhsNote)))
	mux.HandleFunc("/api/xiaohongshu/note-trend", pageProtected("social.xiaohongshu:view", cache24h(h.GetXhsNoteTrend)))
	mux.HandleFunc("/api/xiaohongshu/goods", pageProtected("social.xiaohongshu:view", cache24h(h.GetXhsGoods)))
	mux.HandleFunc("/api/douyin/ops", pageProtected("social.marketing:view", cache24h(h.GetDouyinOps)))
	mux.HandleFunc("/api/douyin-dist/ops", pageProtected("social.marketing:view", cache24h(h.GetDouyinDistOps)))
	mux.HandleFunc("/api/marketing-cost", pageProtected("ecommerce.marketing_cost:view", cache24h(h.GetMarketingCost)))
	mux.HandleFunc("/api/customer/overview", pageProtected("customer.overview:view", cache24h(h.GetCustomerOverview)))
	mux.HandleFunc("/api/customer/comments", pageProtected("customer.comment:view", h.CommentList))
	mux.HandleFunc("/api/customer/comment-options", pageProtected("customer.comment:view", h.CommentOptions))
	mux.HandleFunc("/api/customer/comments/export", pageProtected("customer.comment:view", h.CommentExport))
	// 改名/删除: 客服(view)都能做; 恢复改名/恢复删除: 仅管理员(edit)
	mux.HandleFunc("/api/customer/comments/rename", pageProtected("customer.comment:view", h.CommentRename))
	mux.HandleFunc("/api/customer/comments/delete", pageProtected("customer.comment:view", h.CommentDelete))
	mux.HandleFunc("/api/customer/comments/restore", pageProtected("customer.comment:edit", h.CommentRestore))
	mux.HandleFunc("/api/customer/comments/undelete", pageProtected("customer.comment:edit", h.CommentUndelete))
	// 服务分: 数据量小(每天32店)且每天只导一次, 不走 cache 避免 TTL 跟同步频率错位
	mux.HandleFunc("/api/customer/service-scores", pageProtected("customer.service_score:view", h.GetServiceScores))
	mux.HandleFunc("/api/customer/service-scores/export", pageProtected("customer.service_score:view", h.ServiceScoreExport))
	// 修改: 客服(view)可改(存修正对照表不动RPA原始); 恢复原始: 仅管理员(handler内查 user.manage)
	mux.HandleFunc("/api/customer/service-scores/edit", pageProtected("customer.service_score:view", h.ServiceScoreEdit))
	mux.HandleFunc("/api/customer/service-scores/restore", pageProtected("customer.service_score:view", h.ServiceScoreRestore))
	mux.HandleFunc("/api/s-products", pageAnyProtected(cache24h(h.GetSProducts),
		"ecommerce.store_dashboard:view", "ecommerce.product_dashboard:view",
		"social.store_dashboard:view", "social.product_dashboard:view",
		"offline.store_dashboard:view", "offline.product_dashboard:view",
		"distribution.store_dashboard:view", "distribution.product_dashboard:view",
		"instant_retail.store_dashboard:view", "instant_retail.product_dashboard:view",
	))
	mux.HandleFunc("/api/offline/targets", protected(h.GetOfflineTargets))
	mux.HandleFunc("/api/offline/targets/save", pageProtected("offline.target:edit", h.SaveOfflineTargets))
	mux.HandleFunc("/api/offline/targets/month", protected(h.GetOfflineTargetsByMonth))
	mux.HandleFunc("/api/offline/sales-forecast", pageProtected("offline.sales_forecast:view", h.GetOfflineSalesForecast))
	mux.HandleFunc("/api/offline/sales-forecast/save", pageProtected("offline.sales_forecast:edit", h.SaveOfflineSalesForecast))
	mux.HandleFunc("/api/offline/sales-forecast/clear", pageProtected("offline.sales_forecast:edit", h.ClearOfflineSalesForecast))
	// v1.66.2 重新加回测路由 (但只对当前算法做实时回测, 不再做多算法对比)
	mux.HandleFunc("/api/offline/sales-forecast/backtest-recent", pageProtected("offline.sales_forecast:view", h.GetOfflineSalesForecastBacktestRecent))
	mux.HandleFunc("/api/offline/sales-forecast/sku-trend", pageProtected("offline.sales_forecast:view", h.GetOfflineSalesForecastSKUTrend))
	mux.HandleFunc("/api/webhook/sync-ops", corsHandler(h.SyncOps))
	mux.HandleFunc("/api/webhook/sync-service-score", corsHandler(h.SyncServiceScore))
	mux.HandleFunc("/api/webhook/sync-comment", corsHandler(h.SyncComment))
	mux.HandleFunc("/api/webhook/sync-status", corsHandler(h.SyncStatus))
	mux.HandleFunc("/api/webhook/clear-cache", corsHandler(h.ClearCache))
	mux.HandleFunc("/api/stock/warning", pageProtected("supply_chain.inventory_warning:view", cache24h(h.GetStockWarning)))
	mux.HandleFunc("/api/stock/sync-now", pageProtected("supply_chain.inventory_warning:view", h.SyncStockNow))
	mux.HandleFunc("/api/stock/sync-status", pageProtected("supply_chain.inventory_warning:view", h.SyncStockStatus))
	mux.HandleFunc("/api/supply-chain/dashboard", pageProtected("supply_chain.plan_dashboard:view", cache24h(h.GetSupplyChainDashboard)))
	mux.HandleFunc("/api/supply-chain/monthly-trend", pageProtected("supply_chain.plan_dashboard:view", cache24h(h.GetSupplyChainMonthlyTrend)))
	mux.HandleFunc("/api/supply-chain/purchase-plan", pageProtected("supply_chain.plan_dashboard:view", cache24h(h.GetPurchasePlan)))
	mux.HandleFunc("/api/supply-chain/in-transit-detail", pageProtected("supply_chain.plan_dashboard:view", cache24h(h.GetInTransitDetail)))
	mux.HandleFunc("/api/supply-chain/sync-ys-stock", pageProtected("supply_chain.plan_dashboard:view", h.SyncYSStock))
	mux.HandleFunc("/api/supply-chain/sync-ys-progress", pageProtected("supply_chain.plan_dashboard:view", h.GetSyncYSProgress))
	mux.HandleFunc("/api/supply-chain/qc-alert", pageProtected("supply_chain.qc_alert:view", cache24h(h.GetQCAlert)))
	mux.HandleFunc("/api/supply-chain/qc-alert/arrival", pageProtected("supply_chain.qc_alert:view", h.GetQCArrivalDetail))
	mux.HandleFunc("/api/supply-chain/qc-alert/detail", pageProtected("supply_chain.qc_alert:view", h.GetQCAlertDetail))
	// 原料行情（期货）—— MVP 阶段开放给所有登录用户（不挂 permission，登录即可看）
	mux.HandleFunc("/api/futures/symbols", protected(cache24h(h.GetFuturesSymbols)))
	mux.HandleFunc("/api/futures/quotes", protected(cache5m(h.GetFuturesQuotes)))
	mux.HandleFunc("/api/futures/daily", protected(cache24h(h.GetFuturesDaily)))
	// 分销·客户分析 (v1.29)
	mux.HandleFunc("/api/distribution/customers/list", pageProtected("distribution.customer_list:edit", h.ListDistributionCustomers))
	mux.HandleFunc("/api/distribution/customers/grade", pageProtected("distribution.customer_list:edit", h.SetDistributionCustomerGrade))
	mux.HandleFunc("/api/distribution/customers/grade-batch", pageProtected("distribution.customer_list:edit", h.BatchSetDistributionCustomerGrade))
	mux.HandleFunc("/api/distribution/customer-analysis/kpi", pageProtected("distribution.customer_analysis:view", cache24h(h.DistributionCustomerAnalysisKPI)))
	mux.HandleFunc("/api/distribution/customer-analysis/list", pageProtected("distribution.customer_analysis:view", cache24h(h.DistributionHVCustomerList)))
	mux.HandleFunc("/api/distribution/customer-analysis/monthly", pageProtected("distribution.customer_analysis:view", cache24h(h.DistributionCustomerMonthly)))
	mux.HandleFunc("/api/distribution/customer-analysis/skus", pageProtected("distribution.customer_analysis:view", cache24h(h.DistributionCustomerSkus)))

	// 快递仓储分析 (v0.56)
	mux.HandleFunc("/api/warehouse-flow/overview", pageProtected("supply_chain.logistics_analysis:view", cache24h(h.GetWarehouseFlowOverview)))
	mux.HandleFunc("/api/warehouse-flow/matrix", pageProtected("supply_chain.logistics_analysis:view", cache24h(h.GetWarehouseFlowMatrix)))
	mux.HandleFunc("/api/admin/tasks", adminRoles(h.GetTaskStatus))
	mux.HandleFunc("/api/admin/tasks/run", adminRoles(h.RunManualTask))
	mux.HandleFunc("/api/admin/tasks/running", adminRoles(h.GetRunningTasks))
	mux.HandleFunc("/api/admin/tasks/stop", adminRoles(h.StopManualTask))
	mux.HandleFunc("/api/admin/docs/rpa-mapping", adminRoles(h.GetRPAMapping))
	mux.HandleFunc("/api/admin/docs/db-dict", adminRoles(h.GetDBDictionary))
	mux.HandleFunc("/api/admin/rpa-scan", adminRoles(h.ScanRPAFiles))
	mux.HandleFunc("/api/admin/rpa-scan/refresh", adminRoles(h.RefreshRPAScan))
	mux.HandleFunc("/api/admin/rpa-scan/import", adminRoles(h.ManualImport))
	mux.HandleFunc("/api/admin/rpa-scan/import-progress", adminRoles(h.ImportProgress))

	// 影刀 RPA 触发 (点平台 Tab "立即同步" 按钮)
	mux.HandleFunc("/api/admin/rpa/trigger", adminRoles(h.TriggerRPASync))
	mux.HandleFunc("/api/admin/rpa/batch-trigger", adminRoles(h.BatchTriggerRPASync))
	mux.HandleFunc("/api/admin/rpa/batch-queue", adminRoles(h.GetRPABatchQueue))
	mux.HandleFunc("/api/admin/rpa/job-status", adminRoles(h.GetRPAJobStatus))
	mux.HandleFunc("/api/admin/rpa/active-tasks", adminRoles(h.GetRPAActiveTasks))
	mux.HandleFunc("/api/admin/rpa/platform-mapping", adminRoles(h.GetRPAPlatformMapping))
	mux.HandleFunc("/api/admin/rpa/platform-mapping/update", adminRoles(h.UpdateRPAPlatformMapping))
	// v1.73.0 W1+W2: BI 智能助手 (ask + sessions + messages + feedback)
	// 答案是全公司数据口径且查询不做 scope 过滤, 必须挂权限点限管理层 — 仅 RequireAuth 时任何登录用户可 API 直调拿全公司数据
	mux.HandleFunc("/api/ai-assistant/ask", corsHandler(h.RequirePermission("ai.assistant:use", h.AIAssistantAsk)))
	mux.HandleFunc("/api/ai-assistant/sessions", corsHandler(h.RequirePermission("ai.assistant:use", h.AIAssistantSessions)))
	mux.HandleFunc("/api/ai-assistant/messages", corsHandler(h.RequirePermission("ai.assistant:use", h.AIAssistantMessages)))
	mux.HandleFunc("/api/ai-assistant/feedback", corsHandler(h.RequirePermission("ai.assistant:use", h.AIAssistantFeedback)))

	mux.HandleFunc("/api/admin/yingdao/tasks", adminRoles(h.GetYingDaoTasks))
	mux.HandleFunc("/api/admin/yingdao/sub-apps", adminRoles(h.GetYingDaoSubApps))
	mux.HandleFunc("/api/admin/yingdao/clients", adminRoles(h.GetYingDaoClients))

	// 反馈
	feedbackAdmin := func(next http.HandlerFunc) http.HandlerFunc {
		return corsHandler(h.RequirePermission("feedback.manage", next))
	}
	mux.HandleFunc("/api/feedback", protected(h.SubmitFeedback))
	mux.HandleFunc("/api/feedback/list", feedbackAdmin(h.ListFeedback))
	mux.HandleFunc("/api/feedback/", feedbackAdmin(h.FeedbackByPath))

	// 需求管理（v1.62.0 新增）
	mux.HandleFunc("/api/hesi-bot/approve", protected(h.HesiApprove))
	mux.HandleFunc("/api/hesi-bot/reject-nodes", protected(h.HesiRejectNodes))
	mux.HandleFunc("/api/profile/hesi-pending/sync", protected(h.HesiPendingSync))
	mux.HandleFunc("/api/hesi-bot/approve/queue", protected(h.HesiApprovalQueue))
	mux.HandleFunc("/api/hesi-bot/approve/queue/", protected(h.HesiApprovalQueueItem))
	mux.HandleFunc("/api/requirements", protected(h.SubmitRequirement))
	mux.HandleFunc("/api/requirements/list", protected(h.ListRequirements))
	mux.HandleFunc("/api/requirements/stats", protected(h.RequirementStats))
	mux.HandleFunc("/api/requirements/gantt", protected(h.RequirementGantt))
	mux.HandleFunc("/api/requirements/", protected(h.RequirementByPath))

	// 个人中心
	mux.HandleFunc("/api/user/profile", protected(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.GetProfile(w, r)
		case http.MethodPut:
			h.UpdateProfile(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	}))
	mux.HandleFunc("/api/user/avatar", protected(h.UploadAvatar))

	// v1.59.0 个人中心 → 合思机器人 Tab "我的待审批"
	// v1.63 加 profile.hesi_bot:view 页面级权限, 角色管理可勾选
	mux.HandleFunc("/api/profile/hesi-pending", pageProtected("profile.hesi_bot:view", h.GetMyHesiPending))
	// v1.59.3 管理员查 distinct 审批人列表
	mux.HandleFunc("/api/profile/hesi-approvers", pageProtected("profile.hesi_bot:view", h.GetHesiApprovers))
	// v1.62.x 合思机器人详情/附件 (鉴权: 审批人/提交人/管理员)
	mux.HandleFunc("/api/profile/hesi-flow-detail", pageProtected("profile.hesi_bot:view", h.GetMyHesiFlowDetail))
	mux.HandleFunc("/api/profile/hesi-attachment-urls", pageProtected("profile.hesi_bot:view", h.GetMyHesiAttachmentURLs))
	mux.HandleFunc("/api/profile/hesi-approval-flow", pageProtected("profile.hesi_bot:view", h.GetMyHesiApprovalFlow))
	// v1.60.0 合思机器人规则 CRUD
	mux.HandleFunc("/api/profile/hesi-rules", pageProtected("profile.hesi_bot:view", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.ListMyHesiRules(w, r)
		case http.MethodPost:
			h.CreateMyHesiRule(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	}))
	mux.HandleFunc("/api/profile/hesi-rules/", pageProtected("profile.hesi_bot:view", h.HesiRuleByPath))
	// v1.60.2 个人中心同步钉钉昵称/真名
	mux.HandleFunc("/api/profile/sync-dingtalk", protected(h.SyncMyDingtalk))
	mux.HandleFunc("/api/admin/sync-all-dingtalk-names", adminUsers(h.SyncAllDingtalk))

	// 公告
	noticeAdmin := func(next http.HandlerFunc) http.HandlerFunc {
		return corsHandler(h.RequirePermission("notice.manage", next))
	}
	mux.HandleFunc("/api/notices", protected(h.GetNotices))
	mux.HandleFunc("/api/admin/notices", noticeAdmin(h.AdminListNotices))
	mux.HandleFunc("/api/admin/notices/create", noticeAdmin(h.CreateNotice))
	mux.HandleFunc("/api/admin/notices/", noticeAdmin(h.NoticeByPath))

	// 渠道管理
	channelAdmin := func(next http.HandlerFunc) http.HandlerFunc {
		return corsHandler(h.RequirePermission("channel.manage", next))
	}
	mux.HandleFunc("/api/admin/channels", channelAdmin(h.AdminChannels))
	mux.HandleFunc("/api/admin/channels/sync", channelAdmin(h.SyncChannels))
	mux.HandleFunc("/api/admin/channels/", channelAdmin(h.ChannelByPath))

	// 销售单核对 (跑哥手工对吉客云后台用), 借用 role.manage 权限
	mux.HandleFunc("/api/admin/trade-audit", corsHandler(h.RequirePermission("role.manage", h.AdminTradeAudit)))

	// 同步工具实时日志 (sync-daily-trades 等独立 exe 写固定文件, 这里直接读末尾返回前端)
	mux.HandleFunc("/api/admin/sync-tools/log", corsHandler(h.RequirePermission("role.manage", h.AdminSyncToolLog)))

	// 合思费控 (v1.73.2 hotfix: sync-hesi 每 15min 跑一次, stats 必须 cache15m 跟同频, 否则用户看 24h 旧数据)
	// specifications 是费用类型/规格定义, 几乎不变, 保留 24h
	mux.HandleFunc("/api/hesi/stats", pageProtected("finance.expense:view", cache15m(h.GetHesiStats)))
	mux.HandleFunc("/api/hesi/flows", pageProtected("finance.expense:view", h.GetHesiFlows))
	mux.HandleFunc("/api/hesi/flow-detail", pageProtected("finance.expense:view", h.GetHesiFlowDetail))
	mux.HandleFunc("/api/hesi/specifications", pageProtected("finance.expense:view", cache24h(h.GetHesiSpecifications)))
	mux.HandleFunc("/api/hesi/attachment-urls", pageProtected("finance.expense:view", h.GetHesiAttachmentURLs))
	mux.HandleFunc("/api/hesi/approval-flow", pageProtected("finance.expense:view", h.HesiApprovalFlow))
	mux.HandleFunc("/api/hesi/last-sync", pageProtected("finance.expense:view", h.GetHesiLastSync))

	// 财务报表
	mux.HandleFunc("/api/finance/report", pageProtected("finance.report:view", h.GetFinanceReport))
	mux.HandleFunc("/api/finance/report/trend", pageProtected("finance.report:view", h.GetFinanceReportTrend))
	mux.HandleFunc("/api/finance/report/compare", pageProtected("finance.report:view", h.GetFinanceReportCompare))
	mux.HandleFunc("/api/finance/report/structure", pageProtected("finance.report:view", h.GetFinanceReportStructure))
	mux.HandleFunc("/api/finance/report/subjects", pageProtected("finance.report:view", h.GetFinanceSubjects))
	mux.HandleFunc("/api/finance/report/import/preview", pageProtected("finance.report:import", h.ImportFinancePreview))
	mux.HandleFunc("/api/finance/report/import/confirm", pageProtected("finance.report:import", h.ImportFinanceConfirm))
	mux.HandleFunc("/api/finance/report/export", pageAllProtected(h.ExportFinanceReport, "finance.report:view", "data:export"))

	// SKU动销率 (2026-06-15, 权限位暂只给超管, 未发财务角色)
	mux.HandleFunc("/api/finance/sku-movement", pageProtected("finance.sku_movement:view", h.GetSKUMovement))

	// 凭证查询 (2026-06-16, 实时查用友 YS 凭证, 权限位暂只给超管)
	mux.HandleFunc("/api/finance/voucher/accbooks", pageProtected("finance.voucher:view", h.GetVoucherAccbooks))
	mux.HandleFunc("/api/finance/voucher/list", pageProtected("finance.voucher:view", h.GetVoucherList))

	// 业务预决算报表 (v0.58/v0.59)
	mux.HandleFunc("/api/finance/business-report", pageProtected("finance.report:view", h.GetBusinessReportFinanceLike))
	mux.HandleFunc("/api/finance/business-report/channels", pageProtected("finance.report:view", h.GetBusinessReportChannelsList))

	// 特殊渠道按调拨算销售额(对账页) v0.62, v1.04 修正权限错配
	mux.HandleFunc("/api/special-channel-allot/summary", pageProtected("ecommerce.special_channel_allot:view", h.GetSpecialChannelAllotSummary))
	mux.HandleFunc("/api/special-channel-allot/details", pageProtected("ecommerce.special_channel_allot:view", h.GetSpecialChannelAllotDetails))
	// 价格表维护: 列表谁能看对账页(电商/即时零售任一 view)就能看; 改价单独权限(默认仅超管)
	mux.HandleFunc("/api/special-channel-allot/prices", pageAnyProtected(h.GetChannelPrices, "ecommerce.special_channel_allot:view", "instant_retail.special_channel_allot:view"))
	mux.HandleFunc("/api/special-channel-allot/save-price", pageProtected("ecommerce.special_channel_allot:edit", h.SaveChannelPrice))

	// 受保护的上传文件访问（禁止目录浏览）
	mux.HandleFunc("/api/uploads/", protected(h.ServeUploadFile))

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Server starting on %s", addr)

	go h.StartCleanupRoutines()

	// v1.62.x: 合思审批队列 worker (单 goroutine + 65s 限流 + 批量合并)
	hesiApprovalStop := make(chan struct{})
	go h.StartHesiApprovalWorker(hesiApprovalStop)

	// RPA 文件扫描后台 ticker: 每 5 分钟刷缓存, 让 RPAMonitor 页面打开瞬开
	go handler.StartRPAScanTicker()

	// v1.75.20: 综合看板后台预热 ticker, 让 /api/overview 默认视图始终是热缓存 (秒开)
	go h.StartOverviewPrewarm()

	// 2026-06-15: 合思行车记录缓存启动预热 + 10min 后台刷新, 让审批列表/详情不在请求路径里现拉
	// (重启后冷缓存撞上 = 樊雪娇 35 单审批页加载卡几分钟, 已改 LookupDriveRecord 非阻塞 + 本预热)
	go h.StartDriveRecordPrewarm()

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
