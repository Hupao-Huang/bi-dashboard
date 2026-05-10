package importutil

// paths_test.go — ResolveDataRoot + mapDrivePathToUNC
// 已 Read paths.go 全文 (53 行). const defaultShareRoot=\\172.16.100.10\松鲜鲜资料库

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// === mapDrivePathToUNC (line 42-53) ===
// 源码逻辑:
//   - 输入 trim+clean
//   - 不是 'Z:\' 开头返 "" (case-insensitive 'z:')
//   - shareRoot 从环境变量 BI_SHARE_ROOT 读, 缺则用 defaultShareRoot
//   - 返回 filepath.Join(shareRoot, suffix)

func TestMapDrivePathToUNC_NonZDriveReturnsEmpty(t *testing.T) {
	cases := []string{
		"",
		"C:\\path",
		"D:\\path",
		"/unix/path",
		"relative/path",
		"AB",            // < 3 char
	}
	for _, p := range cases {
		if got := mapDrivePathToUNC(p); got != "" {
			t.Errorf("非 Z: 路径 %q 应返空, got %q", p, got)
		}
	}
}

func TestMapDrivePathToUNC_ZDriveWithDefaultShareRoot(t *testing.T) {
	// 清环境变量, 用 defaultShareRoot
	t.Setenv("BI_SHARE_ROOT", "")
	got := mapDrivePathToUNC(`Z:\信息部\RPA`)
	// 期望: \\172.16.100.10\松鲜鲜资料库\信息部\RPA
	if !strings.Contains(got, "172.16.100.10") {
		t.Errorf("应含默认 share root IP, got %q", got)
	}
	if !strings.Contains(got, "信息部") {
		t.Errorf("应保留 suffix, got %q", got)
	}
}

func TestMapDrivePathToUNC_ZDriveCaseInsensitive(t *testing.T) {
	t.Setenv("BI_SHARE_ROOT", "")
	if got := mapDrivePathToUNC(`Z:\X`); got == "" {
		t.Error("Z:\\ 大写应识别")
	}
	if got := mapDrivePathToUNC(`z:\X`); got == "" {
		t.Error("z:\\ 小写也应识别 (EqualFold)")
	}
}

func TestMapDrivePathToUNC_RespectsEnvOverride(t *testing.T) {
	t.Setenv("BI_SHARE_ROOT", `\\my-server\custom`)
	got := mapDrivePathToUNC(`Z:\sub`)
	if !strings.Contains(got, "my-server") {
		t.Errorf("应用 env 设置的 share root, got %q", got)
	}
	if !strings.Contains(got, "custom") {
		t.Errorf("应保留 env share root path, got %q", got)
	}
}

// === ResolveDataRoot (line 12-40) ===
// 源码逻辑:
//   1. 候选列表: BI_DATA_ROOT 环境 + defaultPath + Z:\→UNC 映射
//   2. 去重 (cleaned path)
//   3. 顺序 stat, 第一个存在且是目录的返回
//   4. 都不存在返 error

func TestResolveDataRoot_PrefersEnvVar(t *testing.T) {
	tmp := t.TempDir() // 一定存在的目录
	t.Setenv("BI_DATA_ROOT", tmp)

	got, err := ResolveDataRoot("/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("env var 是 tmpDir 应成功, got err=%v", err)
	}
	if filepath.Clean(got) != filepath.Clean(tmp) {
		t.Errorf("应优先用 env var, want %s got %s", tmp, got)
	}
}

func TestResolveDataRoot_FallsBackToDefault(t *testing.T) {
	t.Setenv("BI_DATA_ROOT", "")
	tmp := t.TempDir()

	got, err := ResolveDataRoot(tmp)
	if err != nil {
		t.Fatalf("fallback 到 default 应成功, got err=%v", err)
	}
	if filepath.Clean(got) != filepath.Clean(tmp) {
		t.Errorf("default 应命中, want %s got %s", tmp, got)
	}
}

func TestResolveDataRoot_AllInvalidReturnsError(t *testing.T) {
	t.Setenv("BI_DATA_ROOT", "")
	_, err := ResolveDataRoot("/path/that/does/not/exist/at/all")
	if err == nil {
		t.Error("所有候选都不存在应返 error")
	}
}

func TestResolveDataRoot_RejectsFileNotDir(t *testing.T) {
	// info.IsDir() 必须 true (源码 line 34)
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "not_a_dir.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("setup file: %v", err)
	}
	t.Setenv("BI_DATA_ROOT", "")

	_, err := ResolveDataRoot(filePath)
	if err == nil {
		t.Error("候选是文件而非目录应 error")
	}
}

func TestResolveDataRoot_DeduplicatesCandidates(t *testing.T) {
	// env 和 default 是同一个 path → 只 stat 一次, 返回一致
	tmp := t.TempDir()
	t.Setenv("BI_DATA_ROOT", tmp)
	got, err := ResolveDataRoot(tmp) // 重复
	if err != nil {
		t.Fatalf("got err=%v", err)
	}
	if filepath.Clean(got) != filepath.Clean(tmp) {
		t.Error("去重不影响结果")
	}
}
