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
	getTokenURL    = "https://oapi.dingtalk.com/gettoken"
	sendMessageURL = "https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend"
	httpTimeout    = 10 * time.Second
)

// Notifier 钉钉机器人通知客户端
type Notifier struct {
	appKey    string
	appSecret string
	robotCode string

	mu          sync.Mutex
	cachedToken string
	expiresAt   time.Time

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
		appKey:     appKey,
		appSecret:  appSecret,
		robotCode:  robotCode,
		httpClient: &http.Client{Timeout: httpTimeout},
	}
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

// SendText 给指定 unionId 列表推送纯文本消息
// userIds: 钉钉 unionId（users.dingtalk_userid 字段）
// content: 文本内容
func (n *Notifier) SendText(userIds []string, content string) error {
	if len(userIds) == 0 {
		return errors.New("userIds 为空")
	}
	if content == "" {
		return errors.New("content 为空")
	}

	token, err := n.getAccessToken()
	if err != nil {
		return err
	}

	msgParam, _ := json.Marshal(map[string]string{"content": content})
	payload := map[string]interface{}{
		"robotCode": n.robotCode,
		"userIds":   userIds,
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
