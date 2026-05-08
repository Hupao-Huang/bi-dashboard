package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/dingtalk"
	"bi-dashboard/internal/handler"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
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
		log.Println("DingTalk notifier ready (反馈回复将推送给提交人)")
	} else {
		log.Println("DingTalk notifier disabled (未配置 notify_app_key/notify_app_secret)")
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
	}

	mux := http.NewServeMux()

	allowedOrigins := map[string]bool{
		"http://localhost:3000":        true,
		"http://127.0.0.1:3000":        true,
		"http://192.168.200.48:3000":   true,
		"http://songxianxian.local":    true,
		"http://bi.songxianxian.local": true,
	}

	corsHandler := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
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
		}
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
		"finance.department_profit:view", "finance.monthly_profit:view", "finance.product_profit:view",
	))
	mux.HandleFunc("/api/tmall/ops", pageAnyProtected(cache24h(h.GetTmallOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
	))
	mux.HandleFunc("/api/vip/ops", pageAnyProtected(cache24h(h.GetVipOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
	))
	mux.HandleFunc("/api/pdd/ops", pageAnyProtected(cache24h(h.GetPddOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
	))
	mux.HandleFunc("/api/jd/ops", pageAnyProtected(cache24h(h.GetJdOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
	))
	mux.HandleFunc("/api/tmallcs/ops", pageAnyProtected(cache24h(h.GetTmallcsOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
	))
	mux.HandleFunc("/api/feigua", pageProtected("social.feigua:view", cache24h(h.GetFeiguaData)))
	mux.HandleFunc("/api/douyin/ops", pageProtected("social.marketing:view", cache24h(h.GetDouyinOps)))
	mux.HandleFunc("/api/douyin-dist/ops", pageProtected("social.marketing:view", cache24h(h.GetDouyinDistOps)))
	mux.HandleFunc("/api/marketing-cost", pageProtected("ecommerce.marketing_cost:view", cache24h(h.GetMarketingCost)))
	mux.HandleFunc("/api/customer/overview", pageProtected("customer.overview:view", cache24h(h.GetCustomerOverview)))
	mux.HandleFunc("/api/s-products", pageAnyProtected(cache24h(h.GetSProducts),
		"ecommerce.store_dashboard:view", "ecommerce.product_dashboard:view",
	))
	mux.HandleFunc("/api/offline/targets", protected(h.GetOfflineTargets))
	mux.HandleFunc("/api/offline/targets/save", pageProtected("offline.target:edit", h.SaveOfflineTargets))
	mux.HandleFunc("/api/offline/targets/month", protected(h.GetOfflineTargetsByMonth))
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

	// 反馈
	feedbackAdmin := func(next http.HandlerFunc) http.HandlerFunc {
		return corsHandler(h.RequirePermission("feedback.manage", next))
	}
	mux.HandleFunc("/api/feedback", protected(h.SubmitFeedback))
	mux.HandleFunc("/api/feedback/list", feedbackAdmin(h.ListFeedback))
	mux.HandleFunc("/api/feedback/", feedbackAdmin(h.FeedbackByPath))

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

	// 合思费控
	mux.HandleFunc("/api/hesi/stats", pageProtected("finance.expense:view", h.GetHesiStats))
	mux.HandleFunc("/api/hesi/flows", pageProtected("finance.expense:view", h.GetHesiFlows))
	mux.HandleFunc("/api/hesi/flow-detail", pageProtected("finance.expense:view", h.GetHesiFlowDetail))
	mux.HandleFunc("/api/hesi/attachment-urls", pageProtected("finance.expense:view", h.GetHesiAttachmentURLs))

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
