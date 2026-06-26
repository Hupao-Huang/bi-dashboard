package main

// 本机防重复。
//
// BI 服务器版把建单指纹记在数据库 yonbip_submit_log; 独立 exe 离网用不了数据库,
// 改记到 exe 同目录的 po-submit-log.json。10 分钟内同内容(同组织+供应商+日期+明细)再建 → 跳过。
//
// 局限: 只能防"同一台电脑、同一个 exe"的重复。两个人各自的 exe 建同一张单互相不知道,
// 这种跨机重复只能靠人工别重复填(已在交付说明里讲清)。

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const idempWindow = 10 * time.Minute

type submitRecord struct {
	OrderID string `json:"orderId"`
	At      int64  `json:"at"` // unix 秒
}

type idempStore struct {
	mu   sync.Mutex
	path string
	recs map[string]submitRecord // fingerprint → record
}

func newIdempStore(dir string) *idempStore {
	s := &idempStore{
		path: filepath.Join(dir, "po-submit-log.json"),
		recs: map[string]submitRecord{},
	}
	if data, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(data, &s.recs) // 损坏就当空, 不阻塞使用
	}
	return s
}

// fingerprint 由订单内容算出, 同内容→同指纹。
func fingerprint(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:])
}

// recent 返回 (10分钟内是否已提交过, 上次订单号)。
func (s *idempStore) recent(fp string) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[fp]
	if !ok || time.Since(time.Unix(r.At, 0)) > idempWindow {
		return false, ""
	}
	return true, r.OrderID
}

// record 建单成功后立即落账。返回写盘是否成功 —— 写盘失败必须让调用方知道,
// 因为本机内存还记着但重启就丢, 重启后重传会重复建单(不可逆), 须提示用户"别重发"。
// 用 临时文件+原子 rename 落盘: 写一半被杀/断电/被杀毒锁也不会把已有流水截断成残缺 JSON。
func (s *idempStore) record(fp, orderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs[fp] = submitRecord{OrderID: orderID, At: time.Now().Unix()}
	cut := time.Now().Add(-24 * time.Hour).Unix()
	for k, v := range s.recs {
		if v.At < cut {
			delete(s.recs, k) // 清掉一天前的旧记录防文件膨胀
		}
	}
	return s.flushLocked()
}

// flushLocked 原子写盘(临时文件 → fsync → rename 覆盖)。须在持锁时调用。
func (s *idempStore) flushLocked() error {
	data, err := json.MarshalIndent(s.recs, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("建临时文件失败: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("写临时文件失败: %w", err)
	}
	if err := f.Sync(); err != nil { // 落盘到磁盘, 防 OS 缓存里就崩了
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("刷盘失败: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil { // 原子替换: 要么旧文件完好, 要么新文件完整
		os.Remove(tmp)
		return fmt.Errorf("替换正式文件失败: %w", err)
	}
	return nil
}
