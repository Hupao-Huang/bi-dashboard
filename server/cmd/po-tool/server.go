package main

// 本地 HTTP 服务: 给 exe 内嵌的网页用。三个接口:
//   /api/unlock   口令解锁 → 解密用友密钥 → NewClient
//   /api/preview  上传 Excel → 翻译+算价 → 红绿预览(不建单, 存 token)
//   /api/commit   按 token 逐单建用友采购订单(防重 + 全程串行 + 断开即停)

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"bi-dashboard/internal/potoolcrypto"
	"bi-dashboard/internal/yonsuite"

	"github.com/xuri/excelize/v2"
)

const tokenTTL = 30 * time.Minute

type tokenEntry struct {
	orders    []previewOrder
	createdAt time.Time
}

type app struct {
	secretBlob string            // 加密的用友密钥(secret.dat 内容)
	vendors    map[string]string // 供应商名称→编码(本地名单)
	idemp      *idempStore
	html       []byte   // 内嵌前端页面
	allowHosts []string // 允许的 Host 头(只认本机, 防 DNS rebinding/跨站驱动)

	mu       sync.Mutex
	client   *yonsuite.Client // 口令解锁后才有
	tokens   map[string]*tokenEntry
	commitMu sync.Mutex // 串行化 commit: 写用友不可逆, 全程独占, 杜绝并发重复建单
}

func newApp(secretBlob string, vendors map[string]string, idemp *idempStore, html []byte, allowHosts []string) *app {
	return &app{
		secretBlob: secretBlob, vendors: vendors, idemp: idemp, html: html, allowHosts: allowHosts,
		tokens: map[string]*tokenEntry{},
	}
}

func (a *app) getClient() *yonsuite.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client
}

// guardAPI 包裹写接口: 只认 POST + 本机 Host + 同源/无 Origin。
// 挡住本机其它恶意网页通过浏览器跨站 POST 驱动建单(CSRF / DNS rebinding)。
func (a *app) guardAPI(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, map[string]interface{}{"error": "method not allowed"})
			return
		}
		// Host 必须是我们监听的本机地址; DNS rebinding 时浏览器用攻击域名做 Host → 不匹配即拒
		if !a.hostAllowed(r.Host) {
			writeJSON(w, map[string]interface{}{"error": "非法来源(Host)"})
			return
		}
		// 有 Origin 时必须同源(我们自己的页面 fetch 是同源); 跨站页面 Origin 是它自己的域 → 拒
		if origin := r.Header.Get("Origin"); origin != "" && !a.originAllowed(origin) {
			writeJSON(w, map[string]interface{}{"error": "非法来源(Origin)"})
			return
		}
		h(w, r)
	}
}

func (a *app) hostAllowed(host string) bool {
	for _, h := range a.allowHosts {
		if strings.EqualFold(host, h) {
			return true
		}
	}
	return false
}

func (a *app) originAllowed(origin string) bool {
	// origin 形如 http://127.0.0.1:PORT, 去掉 scheme 比 host
	if i := strings.Index(origin, "://"); i >= 0 {
		origin = origin[i+3:]
	}
	return a.hostAllowed(origin)
}

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(a.html)
}

// handleUnlock 用口令解密用友密钥, 成功则建立用友 client(本次进程有效)。
func (a *app) handleUnlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "请输入口令"})
		return
	}
	plain, err := potoolcrypto.Decrypt(a.secretBlob, req.Password)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "口令错误"})
		return
	}
	var sec struct {
		AppKey    string `json:"appkey"`
		AppSecret string `json:"appsecret"`
		BaseURL   string `json:"base_url"`
	}
	if err := json.Unmarshal(plain, &sec); err != nil || sec.AppKey == "" {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "密钥文件异常"})
		return
	}
	a.mu.Lock()
	a.client = yonsuite.NewClient(sec.AppKey, sec.AppSecret, sec.BaseURL)
	a.mu.Unlock()
	writeJSON(w, map[string]interface{}{"ok": true})
}

// handlePreview 上传 Excel → 翻译+算价 → 预览(不建单)。
func (a *app) handlePreview(w http.ResponseWriter, r *http.Request) {
	client := a.getClient()
	if client == nil {
		writeJSON(w, map[string]interface{}{"error": "请先用口令解锁"})
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSON(w, map[string]interface{}{"error": "上传解析失败"})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": "请选择 Excel 文件"})
		return
	}
	defer file.Close()
	xf, err := excelize.OpenReader(file)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": "不是有效的 Excel 文件"})
		return
	}
	defer xf.Close()
	rows, err := parseTemplate(xf)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	previewRows, orders, err := translateAndPrice(rows, client, a.vendors)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	token := newToken()
	a.mu.Lock()
	a.gcTokensLocked()
	a.tokens[token] = &tokenEntry{orders: orders, createdAt: time.Now()}
	a.mu.Unlock()
	writeJSON(w, map[string]interface{}{"token": token, "rows": previewRows, "orders": orders})
}

type commitResult struct {
	OrgCode    string `json:"orgCode"`
	OrgName    string `json:"orgName"`
	VendorName string `json:"vendorName"`
	VouchDate  string `json:"vouchDate"`
	LineCount  int    `json:"lineCount"`
	OK         bool   `json:"ok"`
	Skipped    bool   `json:"skipped"`
	OrderID    string `json:"orderId"`
	Error      string `json:"error,omitempty"`
	Warn       string `json:"warn,omitempty"`
}

// handleCommit 按 token 取预览, 逐单建用友采购订单(写用友=不可逆)。
// 全程持 commitMu 串行: 双击/多标签并发都排队, 杜绝同一单被并发建两遍。
func (a *app) handleCommit(w http.ResponseWriter, r *http.Request) {
	client := a.getClient()
	if client == nil {
		writeJSON(w, map[string]interface{}{"error": "请先用口令解锁"})
		return
	}
	var req struct {
		Token string `json:"token"`
		Force bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeJSON(w, map[string]interface{}{"error": "缺少 token"})
		return
	}

	// 原子"取出并消费"token: 进来就删, 第二个并发的同 token 请求扑空(手快点两下只建一次)。
	// commit 失败/中断也不留 token, 重试须重新预览(重读当前状态, 更安全)。
	a.mu.Lock()
	te := a.tokens[req.Token]
	delete(a.tokens, req.Token)
	a.mu.Unlock()
	if te == nil {
		writeJSON(w, map[string]interface{}{"error": "预览已过期或已提交, 请重新上传"})
		return
	}

	a.commitMu.Lock() // 整个建单循环独占, 写用友不可逆不容并发
	defer a.commitMu.Unlock()

	ctx := r.Context()
	results := make([]commitResult, 0, len(te.orders))
	for _, o := range te.orders {
		res := commitResult{
			OrgCode: o.OrgCode, OrgName: o.OrgName, VendorName: o.VendorName,
			VouchDate: o.VouchDate, LineCount: o.LineCount,
		}
		if ctx.Err() != nil { // 浏览器关了/连接断了 → 停手, 不再往用友写
			res.Error = "已取消(连接断开)"
			results = append(results, res)
			continue
		}
		if o.HasProblem {
			res.Error = "该订单有未解决的问题, 已跳过"
			results = append(results, res)
			continue
		}
		payload := yonsuite.BuildPurchaseOrderPayload(
			yonsuite.POHeaderInput{OrgCode: o.OrgCode, VendorCode: o.VendorCode, VouchDate: o.VouchDate},
			o.Lines,
		)
		fpParts := []string{"po", o.OrgCode, o.VendorCode, o.VouchDate}
		for _, ln := range o.Lines {
			fpParts = append(fpParts, ln.ProductCode,
				strconv.FormatFloat(ln.Qty, 'f', -1, 64),
				strconv.FormatFloat(ln.TaxInclUnitPrice, 'f', -1, 64), ln.ArriveDate)
		}
		fp := fingerprint(fpParts...)
		if !req.Force {
			if dup, prev := a.idemp.recent(fp); dup {
				res.Skipped = true
				res.OrderID = prev
				res.Error = "10分钟内已提交过, 已自动跳过防止重复建单"
				results = append(results, res)
				continue
			}
		}
		id, _, err := client.SavePurchaseOrder(payload)
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		// 建成(不可逆)→ 立即落本机防重流水; 落盘失败必须告诉用户别重发(重启会丢记录)
		res.OK = true
		res.OrderID = id
		if rerr := a.idemp.record(fp, id); rerr != nil {
			res.Warn = "已建单成功, 但本机防重记录没写进磁盘(" + rerr.Error() + ")。若稍后重开工具, 请勿再次提交这张单, 以免重复建单。"
		}
		results = append(results, res)
	}
	writeJSON(w, map[string]interface{}{"results": results})
}

func (a *app) gcTokensLocked() {
	for k, v := range a.tokens {
		if time.Since(v.createdAt) > tokenTTL {
			delete(a.tokens, k)
		}
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
