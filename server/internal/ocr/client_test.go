package ocr

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"testing"
)

func TestParseAmount(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"175.91", 175.91},
		{"-529.00", -529.00},        // 支付截图支出显负数, 保留符号
		{"¥118.00", 118.00},
		{"175.91元", 175.91},
		{"实付款: 1,520.14", 1520.14}, // 含千分位
		{"```json\n{\"实付款\": \"175.91\"}\n```", 175.91},
	}
	for _, c := range cases {
		got, err := ParseAmount(c.in)
		if err != nil {
			t.Fatalf("ParseAmount(%q) err: %v", c.in, err)
		}
		if math.Abs(got-c.want) > 0.001 {
			t.Errorf("ParseAmount(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := ParseAmount("交易成功"); err == nil {
		t.Error("ParseAmount(无数字) 应报错")
	}
}

func TestShrinkJPEG(t *testing.T) {
	// 造一张 2000x4000 的 png
	src := image.NewRGBA(image.Rect(0, 0, 2000, 4000))
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	out, err := ShrinkJPEG(buf.Bytes(), 1280)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Height != 1280 || cfg.Width != 640 {
		t.Errorf("缩后尺寸 = %dx%d, want 640x1280", cfg.Width, cfg.Height)
	}

	// 不放大：长边 ≤ longest 时原尺寸不变
	small := image.NewRGBA(image.Rect(0, 0, 640, 480))
	var buf2 bytes.Buffer
	if err := png.Encode(&buf2, small); err != nil {
		t.Fatal(err)
	}
	out2, err := ShrinkJPEG(buf2.Bytes(), 1280)
	if err != nil {
		t.Fatal(err)
	}
	cfg2, _, err := image.DecodeConfig(bytes.NewReader(out2))
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Width != 640 || cfg2.Height != 480 {
		t.Errorf("不放大: 尺寸 = %dx%d, want 640x480", cfg2.Width, cfg2.Height)
	}
}
