package finance

// filemd5_test.go — FileMD5 函数测试
// 已 Read parser.go line 268-280: open + md5.New() + io.Copy + hex.EncodeToString.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileMD5KnownContent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "hello.txt")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	hash, size, err := FileMD5(path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// "hello world" 的 md5 是 5eb63bbbe01eeed093cb22bb8f5acdc3 (业界已知)
	if hash != "5eb63bbbe01eeed093cb22bb8f5acdc3" {
		t.Errorf("md5('hello world') 应固定 = 5eb63b..., got %s", hash)
	}
	if size != int64(len(content)) {
		t.Errorf("size 应等于文件长度 %d, got %d", len(content), size)
	}
}

func TestFileMD5EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "empty.txt")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	hash, size, err := FileMD5(path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// 空内容的 md5 是 d41d8cd98f00b204e9800998ecf8427e
	if hash != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("空文件 md5 应固定 = d41d8c..., got %s", hash)
	}
	if size != 0 {
		t.Errorf("size 应 0, got %d", size)
	}
}

func TestFileMD5NotFoundReturnsError(t *testing.T) {
	_, _, err := FileMD5("/path/that/does/not/exist.txt")
	if err == nil {
		t.Error("不存在文件应返 err")
	}
}

func TestFileMD5SameContentSameHash(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.txt")
	b := filepath.Join(tmp, "b.txt")
	content := []byte("test content for hash comparison")
	os.WriteFile(a, content, 0644)
	os.WriteFile(b, content, 0644)

	hashA, _, _ := FileMD5(a)
	hashB, _, _ := FileMD5(b)
	if hashA != hashB {
		t.Errorf("同内容文件 hash 必须一致: %s vs %s", hashA, hashB)
	}
}
