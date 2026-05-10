package handler

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	mrand "math/rand"
	"net/http"
	"sync"
	"time"
)

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

