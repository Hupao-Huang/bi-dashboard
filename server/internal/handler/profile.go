package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const avatarDir = `C:\Users\Administrator\bi-dashboard\server\uploads\avatars`

// GetProfile 获取当前用户个人信息
func (h *DashboardHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}

	var realName, avatar, phone, email, dingtalkUserid, passwordHash string
	var lastLoginAt *string
	err := h.DB.QueryRow(`SELECT IFNULL(real_name,''), IFNULL(avatar,''), IFNULL(phone,''), IFNULL(email,''),
		DATE_FORMAT(last_login_at,'%Y-%m-%d %H:%i'), IFNULL(dingtalk_userid,''), IFNULL(password_hash,'') FROM users WHERE id=?`, payload.User.ID).
		Scan(&realName, &avatar, &phone, &email, &lastLoginAt, &dingtalkUserid, &passwordHash)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"username":       payload.User.Username,
		"realName":       realName,
		"avatar":         avatar,
		"phone":          phone,
		"email":          email,
		"lastLoginAt":    lastLoginAt,
		"roles":          payload.Roles,
		"dingtalkBound":  dingtalkUserid != "",
		"hasPassword":    passwordHash != "",
	})
}

// UpdateProfile 更新当前用户个人信息
func (h *DashboardHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, 405, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}

	var req struct {
		RealName *string `json:"realName"`
		Phone    *string `json:"phone"`
		Email    *string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid request")
		return
	}

	sets := []string{}
	args := []interface{}{}
	if req.RealName != nil {
		sets = append(sets, "real_name=?")
		args = append(args, *req.RealName)
	}
	if req.Phone != nil {
		sets = append(sets, "phone=?")
		args = append(args, *req.Phone)
	}
	if req.Email != nil {
		sets = append(sets, "email=?")
		args = append(args, *req.Email)
	}
	if len(sets) == 0 {
		writeError(w, 400, "没有要更新的字段")
		return
	}

	args = append(args, payload.User.ID)
	_, err := h.DB.Exec("UPDATE users SET "+strings.Join(sets, ",")+` WHERE id=?`, args...)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"message": "更新成功"})
}

// UploadAvatar 上传头像
func (h *DashboardHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}

	r.ParseMultipartForm(5 << 20) // 5MB
	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeError(w, 400, "请选择头像文件")
		return
	}
	defer file.Close()

	// 检查文件类型
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".gif" && ext != ".webp" {
		writeError(w, 400, "仅支持 jpg/png/gif/webp 格式")
		return
	}

	// 确保目录存在
	os.MkdirAll(avatarDir, 0755)

	// 生成文件名
	filename := fmt.Sprintf("avatar_%d_%d%s", payload.User.ID, time.Now().UnixMilli(), ext)
	fpath := filepath.Join(avatarDir, filename)

	dst, err := os.Create(fpath)
	if err != nil {
		writeError(w, 500, "保存失败")
		return
	}
	defer dst.Close()
	io.Copy(dst, file)

	// 更新数据库
	avatarURL := "/api/uploads/avatars/" + filename
	_, err = h.DB.Exec("UPDATE users SET avatar=? WHERE id=?", avatarURL, payload.User.ID)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{"avatar": avatarURL, "message": "头像更新成功"})
}
