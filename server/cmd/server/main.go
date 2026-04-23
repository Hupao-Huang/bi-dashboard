package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/handler"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("load config: %v", err)
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

	h := &handler.DashboardHandler{
		DB:               db,
		DingToken:        cfg.DingTalk.WebhookToken,
		DingSecret:       cfg.DingTalk.WebhookSecret,
		DingClientID:     cfg.DingTalk.ClientID,
		DingClientSecret: cfg.DingTalk.ClientSecret,
		HesiAppKey:       cfg.Hesi.AppKey,
		HesiSecret:       cfg.Hesi.Secret,
		WebhookSecret:    cfg.Webhook.Secret,
	}

	mux := http.NewServeMux()

	allowedOrigins := map[string]bool{
		"http://localhost:3000":      true,
		"http://127.0.0.1:3000":      true,
		"http://192.168.200.48:3000": true,
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
				r.Body = http.MaxBytesReader(w, r.Body, 2<<20) // 2MB
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

	// 销售/库存/运营类看板数据一天只变一次，缓存60分钟足够
	// 如需立即生效，调用 /api/webhook/clear-cache（同步脚本自动调用）
	cache5m := func(fn http.HandlerFunc) http.HandlerFunc { return h.WithCache(60*time.Minute, fn) }

	mux.HandleFunc("/api/overview", pageAnyProtected(cache5m(h.GetOverview),
		"overview:view", "finance.overview:view", "finance.monthly_profit:view",
	))
	mux.HandleFunc("/api/department", pageAnyProtected(cache5m(h.GetDepartmentDetail),
		"ecommerce.store_preview:view", "ecommerce.store_dashboard:view", "ecommerce.product_dashboard:view",
		"social.store_preview:view", "social.store_dashboard:view", "social.product_dashboard:view",
		"offline.store_preview:view", "offline.store_dashboard:view", "offline.product_dashboard:view",
		"distribution.store_preview:view", "distribution.store_dashboard:view", "distribution.product_dashboard:view",
		"finance.department_profit:view", "finance.monthly_profit:view", "finance.product_profit:view",
	))
	mux.HandleFunc("/api/tmall/ops", pageAnyProtected(cache5m(h.GetTmallOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
	))
	mux.HandleFunc("/api/vip/ops", pageAnyProtected(cache5m(h.GetVipOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
	))
	mux.HandleFunc("/api/pdd/ops", pageAnyProtected(cache5m(h.GetPddOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
	))
	mux.HandleFunc("/api/jd/ops", pageAnyProtected(cache5m(h.GetJdOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
	))
	mux.HandleFunc("/api/tmallcs/ops", pageAnyProtected(cache5m(h.GetTmallcsOps),
		"ecommerce.store_dashboard:view", "social.store_dashboard:view", "offline.store_dashboard:view", "distribution.store_dashboard:view",
	))
	mux.HandleFunc("/api/feigua", pageProtected("social.feigua:view", cache5m(h.GetFeiguaData)))
	mux.HandleFunc("/api/douyin/ops", pageProtected("social.marketing:view", cache5m(h.GetDouyinOps)))
	mux.HandleFunc("/api/douyin-dist/ops", pageProtected("social.marketing:view", cache5m(h.GetDouyinDistOps)))
	mux.HandleFunc("/api/marketing-cost", pageProtected("ecommerce.marketing_cost:view", cache5m(h.GetMarketingCost)))
	mux.HandleFunc("/api/customer/overview", pageProtected("customer.overview:view", cache5m(h.GetCustomerOverview)))
	mux.HandleFunc("/api/s-products", pageAnyProtected(cache5m(h.GetSProducts),
		"ecommerce.store_dashboard:view", "ecommerce.product_dashboard:view",
	))
	mux.HandleFunc("/api/channels", pageAnyProtected(cache5m(h.GetChannels),
		"ecommerce.store_preview:view", "ecommerce.store_dashboard:view",
		"social.store_preview:view", "social.store_dashboard:view",
		"offline.store_preview:view", "offline.store_dashboard:view",
		"distribution.store_preview:view", "distribution.store_dashboard:view",
	))
	mux.HandleFunc("/api/offline/targets", protected(h.GetOfflineTargets))
	mux.HandleFunc("/api/offline/targets/save", pageProtected("offline.target:edit", h.SaveOfflineTargets))
	mux.HandleFunc("/api/offline/targets/month", protected(h.GetOfflineTargetsByMonth))
	mux.HandleFunc("/api/webhook/sync-ops", corsHandler(h.SyncOps))
	mux.HandleFunc("/api/webhook/sync-status", corsHandler(h.SyncStatus))
	mux.HandleFunc("/api/webhook/clear-cache", corsHandler(h.ClearCache))
	mux.HandleFunc("/api/stock/warning", pageProtected("supply_chain.inventory_warning:view", cache5m(h.GetStockWarning)))
	mux.HandleFunc("/api/supply-chain/dashboard", pageProtected("supply_chain.plan_dashboard:view", cache5m(h.GetSupplyChainDashboard)))
	mux.HandleFunc("/api/supply-chain/monthly-trend", pageProtected("supply_chain.plan_dashboard:view", cache5m(h.GetSupplyChainMonthlyTrend)))
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
	mux.HandleFunc("/api/feedback/my", protected(h.MyFeedback))
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
	mux.HandleFunc("/api/finance/report/imports", pageProtected("finance.report:view", h.GetFinanceImportLogs))
	mux.HandleFunc("/api/finance/report/import", pageProtected("finance.report:import", h.ImportFinanceReport))
	mux.HandleFunc("/api/finance/report/export", pageProtected("finance.report:view", h.ExportFinanceReport))

	// 受保护的上传文件访问（禁止目录浏览）
	mux.HandleFunc("/api/uploads/", protected(h.ServeUploadFile))

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Server starting on %s", addr)

	go h.StartCleanupRoutines()

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}
