package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const uploadsRootDir = `C:\Users\Administrator\bi-dashboard\server\uploads`

// ServeUploadFile 受保护的上传文件访问，禁止目录浏览和越权路径。
func (h *DashboardHandler) ServeUploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	relPath := strings.TrimPrefix(r.URL.Path, "/api/uploads/")
	relPath = strings.TrimSpace(strings.TrimPrefix(relPath, "/"))
	if relPath == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	cleanRelPath := filepath.Clean(relPath)
	if cleanRelPath == "." || strings.HasPrefix(cleanRelPath, "..") {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	root := filepath.Clean(uploadsRootDir)
	fullPath := filepath.Clean(filepath.Join(root, cleanRelPath))
	rootPrefix := strings.ToLower(root + string(os.PathSeparator))
	fullLower := strings.ToLower(fullPath)
	if fullLower != strings.ToLower(root) && !strings.HasPrefix(fullLower, rootPrefix) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	http.ServeFile(w, r, fullPath)
}
