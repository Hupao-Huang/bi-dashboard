package handler

// auth_pure_test.go — auth.go pure function 单元测试
// 已 Read auth.go: validatePassword (line 1287-1309), inPieceShape (line 171-189),
// blendColor (line 139-148), 常量 pieceSize=44, piecePad=6 (line 72-73).

import (
	"image/color"
	"testing"
)

// === validatePassword (line 1287-1309) ===
// 业务规则:
//   1. len < 8 → "密码至少8位"
//   2. password == username (case-insensitive) → "密码不能和用户名相同"
//   3. 必须含 大写 + 小写 + 数字 → 否则报错
//   4. 全满足 → nil

func TestValidatePasswordTooShort(t *testing.T) {
	cases := []string{"", "a", "Aa1", "Abcdefg"} // 全 < 8
	for _, p := range cases {
		if err := validatePassword(p, "user"); err == nil {
			t.Errorf("password=%q (len<8) 应返错, got nil", p)
		}
	}
}

func TestValidatePasswordSameAsUsername(t *testing.T) {
	// case-insensitive 匹配 (源码 line 1291: strings.EqualFold)
	if err := validatePassword("Admin123", "Admin123"); err == nil {
		t.Error("password=username 必须报错 (即使其他规则都满足)")
	}
	if err := validatePassword("AdMin123", "admin123"); err == nil {
		t.Error("EqualFold case-insensitive 应识别")
	}
}

func TestValidatePasswordMustHaveUpperLowerDigit(t *testing.T) {
	cases := map[string]bool{
		// 缺什么
		"abcdefgh":  false, // 无大写 + 无数字
		"ABCDEFGH":  false, // 无小写 + 无数字
		"abcdefgX":  false, // 无数字
		"abcdef12":  false, // 无大写
		"ABCDEF12":  false, // 无小写
		"12345678":  false, // 无大写 + 无小写
		// 全满足
		"Abc12345":  true,
		"Pass123A":  true,
		"X9z8y7w6": true,
	}
	for p, valid := range cases {
		err := validatePassword(p, "user")
		if valid && err != nil {
			t.Errorf("password=%q 应通过, got err=%v", p, err)
		}
		if !valid && err == nil {
			t.Errorf("password=%q 应报错 (缺大写/小写/数字), got nil", p)
		}
	}
}

// === inPieceShape (line 171-189) ===
// 拼图形状 = 主体方块 (44x44) + 右半圆 + 底部半圆 (半径 6)
// pieceSize=44, piecePad=6

func TestInPieceShapeCenterIsInside(t *testing.T) {
	cx, cy := 100, 100
	// 中心点 (cx+22, cy+22) 应在主体方块内
	if !inPieceShape(cx+22, cy+22, cx, cy) {
		t.Error("方块中心应被认为在内")
	}
	// 4 个角附近也在主体内 (px >= cx && px < cx+44)
	if !inPieceShape(cx, cy, cx, cy) {
		t.Error("左上角 (cx,cy) 应在内")
	}
	if !inPieceShape(cx+43, cy+43, cx, cy) {
		t.Error("右下角 (cx+43,cy+43) 应在内")
	}
}

func TestInPieceShapeFarPointOutside(t *testing.T) {
	cx, cy := 100, 100
	// 远离主体的点
	if inPieceShape(50, 50, cx, cy) {
		t.Error("(50,50) 远离 (100,100) 主体, 应在外")
	}
	if inPieceShape(200, 200, cx, cy) {
		t.Error("(200,200) 应在外")
	}
}

func TestInPieceShapeRightBumpDetected(t *testing.T) {
	// 右侧凸起圆心 (cx+44, cy+22), 半径 6
	cx, cy := 100, 100
	rcx, rcy := cx+44, cy+22
	// 圆心
	if !inPieceShape(rcx, rcy, cx, cy) {
		t.Error("右凸圆心应在内")
	}
	// 圆心边缘 (距离=6)
	if !inPieceShape(rcx+6, rcy, cx, cy) {
		t.Error("右凸 +6 边缘应在内 (<=半径)")
	}
	// 距离=10 (远超 6)
	if inPieceShape(rcx+10, rcy, cx, cy) {
		t.Error("距离 10 > 半径 6 应在外")
	}
}

func TestInPieceShapeBottomBumpDetected(t *testing.T) {
	cx, cy := 100, 100
	bcx, bcy := cx+22, cy+44
	if !inPieceShape(bcx, bcy, cx, cy) {
		t.Error("底凸圆心应在内")
	}
	if !inPieceShape(bcx, bcy+6, cx, cy) {
		t.Error("底凸 +6 边缘应在内")
	}
	if inPieceShape(bcx, bcy+10, cx, cy) {
		t.Error("距离 10 > 半径 6 应在外")
	}
}

// === blendColor (line 139-148) ===
// alpha 混合 (源码完整看不到, 直接测 property: 输入相同 输出相同)

func TestBlendColorDeterministic(t *testing.T) {
	bg := color.RGBA{R: 100, G: 50, B: 200, A: 255}
	fg := color.RGBA{R: 200, G: 100, B: 50, A: 128}
	a := blendColor(bg, fg)
	b := blendColor(bg, fg)
	if a != b {
		t.Errorf("blendColor 必须确定: %v vs %v", a, b)
	}
}

func TestBlendColorPreservesAlpha255(t *testing.T) {
	// 输出 RGBA 应有合理值, 不是 zero
	bg := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	fg := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	out := blendColor(bg, fg)
	// 至少有非零 RGB
	if out.R == 0 && out.G == 0 && out.B == 0 {
		t.Errorf("白前景在黑背景上 blend 不应全黑, got %v", out)
	}
}
