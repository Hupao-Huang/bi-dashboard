// Package dingtalk 钉钉机器人主动通知（chatbotToOne）
//
// 用于：BI 看板事件触发时（如管理员回复反馈）主动 push 消息给指定用户。
// 走的是钉钉企业内部应用 API，不是自定义机器人 webhook。
//
// 凭证：复用 hermes-agent 钉钉应用 (AppKey + AppSecret)，需在钉钉后台
// 给该应用启用 "企业机器人主动消息" 权限。
//
// 接收人：传入用户的 unionId（users 表 dingtalk_userid 字段）。
package dingtalk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	getTokenURL      = "https://oapi.dingtalk.com/gettoken"
	getByUnionIDURL  = "https://oapi.dingtalk.com/topapi/user/getbyunionid"
	sendMessageURL   = "https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend"
	httpTimeout      = 10 * time.Second
)

// Notifier 钉钉机器人通知客户端
type Notifier struct {
	appKey    string
	appSecret string
	robotCode string

	mu           sync.Mutex
	cachedToken  string
	expiresAt    time.Time
	staffIDCache map[string]string // unionId → staffId

	httpClient *http.Client
}

// NewNotifier 构造通知客户端，凭证留空时返回 nil（调用方需判空）
func NewNotifier(appKey, appSecret, robotCode string) *Notifier {
	if appKey == "" || appSecret == "" {
		return nil
	}
	if robotCode == "" {
		robotCode = appKey
	}
	return &Notifier{
		appKey:       appKey,
		appSecret:    appSecret,
		robotCode:    robotCode,
		staffIDCache: make(map[string]string),
		httpClient:   &http.Client{Timeout: httpTimeout},
	}
}

// resolveStaffID 把 UnionId 转为企业内 staffId, 失败回原值由调用方判断
// 缓存命中常驻进程内（unionId/staffId 一对一不会变）
func (n *Notifier) resolveStaffID(unionID string) (string, error) {
	if unionID == "" {
		return "", errors.New("unionID is empty")
	}
	n.mu.Lock()
	if sid, ok := n.staffIDCache[unionID]; ok {
		n.mu.Unlock()
		return sid, nil
	}
	n.mu.Unlock()

	token, err := n.getAccessToken()
	if err != nil {
		return "", err
	}

	body, _ := json.Marshal(map[string]string{"unionid": unionID})
	resp, err := n.httpClient.Post(
		getByUnionIDURL+"?access_token="+token,
		"application/json", bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("getbyunionid request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Result  struct {
			UserID string `json:"userid"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("getbyunionid parse: %w (body=%s)", err, string(respBody))
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("getbyunionid errcode=%d msg=%s", result.ErrCode, result.ErrMsg)
	}
	if result.Result.UserID == "" {
		return "", fmt.Errorf("getbyunionid empty userid for unionID=%s", unionID)
	}

	n.mu.Lock()
	n.staffIDCache[unionID] = result.Result.UserID
	n.mu.Unlock()
	return result.Result.UserID, nil
}

// getAccessToken 获取/刷新 access_token (缓存 7000s, 留 200s 余量)
func (n *Notifier) getAccessToken() (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.cachedToken != "" && time.Now().Before(n.expiresAt) {
		return n.cachedToken, nil
	}

	q := url.Values{}
	q.Set("appkey", n.appKey)
	q.Set("appsecret", n.appSecret)
	resp, err := n.httpClient.Get(getTokenURL + "?" + q.Encode())
	if err != nil {
		return "", fmt.Errorf("gettoken request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("gettoken parse: %w (body=%s)", err, string(body))
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("gettoken errcode=%d msg=%s", result.ErrCode, result.ErrMsg)
	}

	n.cachedToken = result.AccessToken
	expires := result.ExpiresIn - 200
	if expires < 60 {
		expires = 60
	}
	n.expiresAt = time.Now().Add(time.Duration(expires) * time.Second)
	return n.cachedToken, nil
}

// SendText 给指定 UnionId 列表推送纯文本消息
// unionIDs: 钉钉 UnionId（users.dingtalk_userid 字段, 内部会自动转为 staffId）
// content: 文本内容
func (n *Notifier) SendText(unionIDs []string, content string) error {
	if len(unionIDs) == 0 {
		return errors.New("unionIDs 为空")
	}
	if content == "" {
		return errors.New("content 为空")
	}

	// UnionId → staffId 转换
	staffIDs := make([]string, 0, len(unionIDs))
	for _, uid := range unionIDs {
		sid, err := n.resolveStaffID(uid)
		if err != nil {
			log.Printf("[dingtalk-notify] resolve unionID=%s failed: %v", uid, err)
			continue
		}
		staffIDs = append(staffIDs, sid)
	}
	if len(staffIDs) == 0 {
		return errors.New("无有效 staffId（UnionId 转换全部失败）")
	}

	token, err := n.getAccessToken()
	if err != nil {
		return err
	}

	msgParam, _ := json.Marshal(map[string]string{"content": content})
	payload := map[string]interface{}{
		"robotCode": n.robotCode,
		"userIds":   staffIDs,
		"msgKey":    "sampleText",
		"msgParam":  string(msgParam),
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", sendMessageURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sendMessage request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("sendMessage status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ProcessQueryKey string   `json:"processQueryKey"`
		InvalidStaffIds []string `json:"invalidStaffIdList"`
		FlowControlList []string `json:"flowControlledStaffIdList"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("sendMessage parse: %w (body=%s)", err, string(respBody))
	}
	if len(result.InvalidStaffIds) > 0 {
		log.Printf("[dingtalk-notify] invalid staff ids: %v", result.InvalidStaffIds)
	}
	if len(result.FlowControlList) > 0 {
		log.Printf("[dingtalk-notify] flow controlled: %v", result.FlowControlList)
	}
	return nil
}

// SendTextAsync 异步发送，失败只打日志不阻塞主流程
func (n *Notifier) SendTextAsync(userIds []string, content string) {
	go func() {
		if err := n.SendText(userIds, content); err != nil {
			log.Printf("[dingtalk-notify] send failed: %v", err)
		}
	}()
}
