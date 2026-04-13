package importutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultShareRoot = `\\172.16.100.10\松鲜鲜资料库`

func ResolveDataRoot(defaultPath string) (string, error) {
	candidates := []string{}
	if envPath := strings.TrimSpace(os.Getenv("BI_DATA_ROOT")); envPath != "" {
		candidates = append(candidates, envPath)
	}
	candidates = append(candidates, defaultPath)
	if uncPath := mapDrivePathToUNC(defaultPath); uncPath != "" {
		candidates = append(candidates, uncPath)
	}

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("data root not accessible")
}

func mapDrivePathToUNC(path string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if len(cleaned) < 3 || !strings.EqualFold(cleaned[:3], `Z:\`) {
		return ""
	}
	shareRoot := strings.TrimSpace(os.Getenv("BI_SHARE_ROOT"))
	if shareRoot == "" {
		shareRoot = defaultShareRoot
	}
	suffix := strings.TrimPrefix(cleaned, `Z:\`)
	return filepath.Join(shareRoot, suffix)
}
