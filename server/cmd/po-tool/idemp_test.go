package main

import (
	"testing"
	"time"
)

func TestIdempRecentRecordAndReload(t *testing.T) {
	dir := t.TempDir()
	s := newIdempStore(dir)
	fp := fingerprint("po", "org", "vendor", "2026-06-17", "prod", "1", "100", "")

	if dup, _ := s.recent(fp); dup {
		t.Fatal("新指纹不应命中防重")
	}
	if err := s.record(fp, "ORDER123"); err != nil {
		t.Fatalf("record 落盘应成功, got %v", err)
	}
	if dup, id := s.recent(fp); !dup || id != "ORDER123" {
		t.Fatalf("记录后应命中并返回订单号, got dup=%v id=%s", dup, id)
	}
	// 模拟关掉 exe 再打开: 从 po-submit-log.json 读回, 防重应持久
	s2 := newIdempStore(dir)
	if dup, id := s2.recent(fp); !dup || id != "ORDER123" {
		t.Fatalf("重启后应从文件读回防重记录, got dup=%v id=%s", dup, id)
	}
}

func TestIdempWindowExpiry(t *testing.T) {
	dir := t.TempDir()
	s := newIdempStore(dir)
	fp := fingerprint("a")
	s.recs[fp] = submitRecord{OrderID: "OLD", At: time.Now().Add(-11 * time.Minute).Unix()}
	if dup, _ := s.recent(fp); dup {
		t.Fatal("超过 10 分钟窗口不应再命中(允许重建)")
	}
}

func TestFingerprintStableAndDistinct(t *testing.T) {
	if fingerprint("x", "y", "z") != fingerprint("x", "y", "z") {
		t.Fatal("同内容指纹必须稳定")
	}
	if fingerprint("x", "y", "z") == fingerprint("x", "y", "z2") {
		t.Fatal("不同内容指纹必须不同")
	}
}
