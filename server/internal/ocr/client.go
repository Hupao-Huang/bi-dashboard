package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
)

const dashScopeEndpoint = "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"
const ocrPrompt = "这是一张付款/支付截图。只返回本次实际支付的总金额数字，例如175.91。不要手续费、余额、原价、优惠、红包、运费、数量等其它数字。只输出一个数字。"

var amountRe = regexp.MustCompile(`-?\d[\d,]*\.?\d*`)

// ParseAmount 从模型返回文本抠出第一个金额数字, 保留正负号(支付截图支出常为负).
func ParseAmount(raw string) (float64, error) {
	m := amountRe.FindString(raw)
	if m == "" {
		return 0, fmt.Errorf("未找到金额数字: %q", raw)
	}
	m = strings.ReplaceAll(m, ",", "")
	v, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return 0, fmt.Errorf("金额解析失败 %q: %w", m, err)
	}
	return v, nil
}

// ShrinkJPEG 解码 png/jpg, 长边 > longest 时等比缩小, 重编码为 JPEG(q88).
func ShrinkJPEG(img []byte, longest int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(img))
	if err != nil {
		return nil, fmt.Errorf("解码图片失败: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	long := w
	if h > w {
		long = h
	}
	dw, dh := w, h
	if long > longest {
		scale := float64(longest) / float64(long)
		dw = int(float64(w) * scale)
		dh = int(float64(h) * scale)
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 88}); err != nil {
		return nil, fmt.Errorf("编码 JPEG 失败: %w", err)
	}
	return out.Bytes(), nil
}

// RecognizePaymentAmount 缩图→调 qwen-vl-ocr→ParseAmount。amount 保留符号(调用方取绝对值)。
func RecognizePaymentAmount(ctx context.Context, apiKey string, img []byte) (amount float64, raw string, err error) {
	small, err := ShrinkJPEG(img, 1280)
	if err != nil {
		return 0, "", err
	}
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(small)
	body := map[string]any{
		"model": "qwen-vl-ocr",
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
				map[string]any{"type": "text", "text": ocrPrompt},
			},
		}},
	}
	bs, err := json.Marshal(body)
	if err != nil {
		return 0, "", fmt.Errorf("序列化OCR请求体失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", dashScopeEndpoint, bytes.NewReader(bs))
	if err != nil {
		return 0, "", fmt.Errorf("构造OCR请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	// 阿里云直连, 不走 env 代理
	client := &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	var out struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, "", err
	}
	if resp.StatusCode != 200 || len(out.Choices) == 0 {
		return 0, "", fmt.Errorf("OCR调用失败 http %d: %s %s", resp.StatusCode, out.Error.Code, out.Error.Message)
	}
	raw = out.Choices[0].Message.Content
	amt, err := ParseAmount(raw)
	return amt, raw, err
}
