package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/havline/havline/internal/backup"
	"github.com/havline/havline/internal/settings"
)

type BackupHandler struct {
	svc      *backup.Service
	settings *settings.Store
}

func NewBackupHandler(svc *backup.Service, settingsStore *settings.Store) *BackupHandler {
	return &BackupHandler{svc: svc, settings: settingsStore}
}

func (h *BackupHandler) Export(w http.ResponseWriter, r *http.Request) {
	path, err := h.svc.Export(r.Context())
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "创建备份失败")
		return
	}
	defer os.Remove(path)

	file, err := os.Open(path)
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取备份失败")
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(path))
	_, _ = io.Copy(w, file)
}

func (h *BackupHandler) Restore(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(r, w, http.StatusBadRequest, "上传文件无效")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(r, w, http.StatusBadRequest, "请选择备份文件")
		return
	}
	defer file.Close()

	// 上传文件名完全由客户端控制：不能拼进临时路径（../ 可逃出临时目录），交给 CreateTemp 生成
	out, err := os.CreateTemp("", "havline-restore-*.tar.gz")
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "保存上传文件失败")
		return
	}
	tmpPath := out.Name()
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		_ = os.Remove(tmpPath)
		writeError(r, w, http.StatusInternalServerError, "保存上传文件失败")
		return
	}
	out.Close()

	if err := h.svc.Restore(r.Context(), tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	_ = os.Remove(tmpPath)
	writeJSON(w, http.StatusOK, map[string]string{"message": "备份已恢复，建议重启 Havline"})
}

// ListArchives 列出留在磁盘上的备份（自动备份 + 手动「立即备份」）。
// 与「立即下载」不同：这些文件长期留在 <DataDir>/backups 下。
func (h *BackupHandler) ListArchives(w http.ResponseWriter, r *http.Request) {
	archives, err := h.svc.List()
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取备份列表失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"archives": archives,
		"keep":     backup.KeepCount(r.Context(), h.settings),
	})
}

// CreateArchive 立即备份一份并保留在服务器上，顺带按保留份数剪枝
func (h *BackupHandler) CreateArchive(w http.ResponseWriter, r *http.Request) {
	path, err := h.svc.Export(r.Context())
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "创建备份失败", err)
		return
	}
	name := filepath.Base(path)
	// 写失败的备份不能留在列表里冒充可用备份
	if _, err := h.svc.Verify(r.Context(), path); err != nil {
		_ = h.svc.Remove(name)
		writeError(r, w, http.StatusInternalServerError, "备份校验失败："+err.Error(), err)
		return
	}
	removed, err := h.svc.Prune(backup.KeepCount(r.Context(), h.settings))
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "备份已创建，但清理旧备份失败："+err.Error(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "removed": removed})
}

// DownloadArchive 下载指定备份；用 ServeContent 支持断点续传
func (h *BackupHandler) DownloadArchive(w http.ResponseWriter, r *http.Request) {
	path, err := h.svc.Path(r.PathValue("name"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeError(r, w, http.StatusNotFound, "备份不存在")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取备份失败", err)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(path))
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

// RestoreArchive 从指定备份恢复。恢复会覆盖数据库与证书，前端必须二次确认
func (h *BackupHandler) RestoreArchive(w http.ResponseWriter, r *http.Request) {
	path, err := h.svc.Path(r.PathValue("name"))
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	// 恢复前先校验：半个包不能拿去覆盖现有数据
	if _, err := h.svc.Verify(r.Context(), path); err != nil {
		writeError(r, w, http.StatusBadRequest, "该备份不可用："+err.Error(), err)
		return
	}
	if err := h.svc.Restore(r.Context(), path); err != nil {
		writeError(r, w, http.StatusBadRequest, err.Error(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "备份已恢复，建议重启 Havline"})
}

// DeleteArchive 删除指定备份
func (h *BackupHandler) DeleteArchive(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Remove(r.PathValue("name")); err != nil {
		if err.Error() == "备份不存在" {
			writeError(r, w, http.StatusNotFound, err.Error())
			return
		}
		writeError(r, w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
