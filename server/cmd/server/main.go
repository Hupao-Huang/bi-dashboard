package main

import (
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/dingtalk"
	"bi-dashboard/internal/handler"
	"bi-dashboard/internal/yingdao"

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
	mux.HandleFunc("/api/admin/roles", adminRoles(h.AdminRoles))
	mux.HandleFunc("/api/admin/roles/", adminRoles(h.AdminRoleByPath))
	mux.HandleFunc("/api/audit/page-view", protected(h.AuditLogPageView))
	mux.HandleFunc("/api/admin/audit-logs", adminMeta(h.AdminAuditLogs))
	mux.HandleFunc("/api/admin/pending-counts", protected(h.AdminPendingCounts))

	// T-1 数据看板：昨天/前天/上月数据永远不变，缓存 24 小时
	// 同步脚本完成后会调 ClearCacheByPrefix 主动清除（详见 supply_chain.go / stock.go）
	// 历史背景：曾命名 cache5m 但 TTL 为 60min，现统一为 cache24h（v0.56.7）
	cache24h := func(fn http.HandlerFunc) http.HandlerFunc { return h.WithCache(24*time.Hour, fn) }

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
	mux.HandleFunc("/api/douyin/ops", pageProtected("social.marketing:view", cache24h(h.GetDouyinOps)))
	mux.HandleFunc("/api/douyin-dist/ops", pageProtected("social.marketing:view", cache24h(h.GetDouyinDistOps)))
	mux.HandleFunc("/api/marketing-cost", pageProtected("ecommerce.marketing_cost:view", cache24h(h.GetMarketingCost)))
	mux.HandleFunc("/api/customer/overview", pageProtected("customer.overview:view", cache24h(h.GetCustomerOverview)))
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
	// 原料行情（期货）—— MVP 阶段开放给所有登录用户（不挂 permission，登录即可看）
	mux.HandleFunc("/api/futures/symbols", protected(cache24h(h.GetFuturesSymbols)))
	mux.HandleFunc("/api/futures/quotes", protected(cache24h(h.GetFuturesQuotes)))
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
	mux.HandleFunc("/api/admin/yingdao/tasks", adminRoles(h.GetYingDaoTasks))
	mux.HandleFunc("/api/admin/yingdao/sub-apps", adminRoles(h.GetYingDaoSubApps))

	// 反馈
	feedbackAdmin := func(next http.HandlerFunc) http.HandlerFunc {
		return corsHandler(h.RequirePermission("feedback.manage", next))
	}
	mux.HandleFunc("/api/feedback", protected(h.SubmitFeedback))
	mux.HandleFunc("/api/feedback/list", feedbackAdmin(h.ListFeedback))
	mux.HandleFunc("/api/feedback/", feedbackAdmin(h.FeedbackByPath))

	// 需求管理（v1.62.0 新增）
	mux.HandleFunc("/api/hesi-bot/approve", protected(h.HesiApprove))
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

	// 合思费控
	mux.HandleFunc("/api/hesi/stats", pageProtected("finance.expense:view", h.GetHesiStats))
	mux.HandleFunc("/api/hesi/flows", pageProtected("finance.expense:view", h.GetHesiFlows))
	mux.HandleFunc("/api/hesi/flow-detail", pageProtected("finance.expense:view", h.GetHesiFlowDetail))
	mux.HandleFunc("/api/hesi/specifications", pageProtected("finance.expense:view", h.GetHesiSpecifications))
	mux.HandleFunc("/api/hesi/attachment-urls", pageProtected("finance.expense:view", h.GetHesiAttachmentURLs))
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

	// 业务预决算报表 (v0.58/v0.59)
	mux.HandleFunc("/api/finance/business-report", pageProtected("finance.report:view", h.GetBusinessReportFinanceLike))
	mux.HandleFunc("/api/finance/business-report/channels", pageProtected("finance.report:view", h.GetBusinessReportChannelsList))

	// 特殊渠道按调拨算销售额(对账页) v0.62, v1.04 修正权限错配
	mux.HandleFunc("/api/special-channel-allot/summary", pageProtected("ecommerce.special_channel_allot:view", h.GetSpecialChannelAllotSummary))
	mux.HandleFunc("/api/special-channel-allot/details", pageProtected("ecommerce.special_channel_allot:view", h.GetSpecialChannelAllotDetails))

	// 受保护的上传文件访问（禁止目录浏览）
	mux.HandleFunc("/api/uploads/", protected(h.ServeUploadFile))

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Server starting on %s", addr)

	go h.StartCleanupRoutines()

	// v1.62.x: 合思审批队列 worker (单 goroutine + 65s 限流 + 批量合并)
	hesiApprovalStop := make(chan struct{})
	go h.StartHesiApprovalWorker(hesiApprovalStop)

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
