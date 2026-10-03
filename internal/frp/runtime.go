package frp

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	githubReleasesURL = "https://api.github.com/repos/fatedier/frp/releases"
	maxArchiveSize    = 150 << 20
)

var downloadProxyOptions = map[string]struct{}{
	"":                          {},
	"https://gh-proxy.org/":     {},
	"https://v4.gh-proxy.org/":  {},
	"https://cdn.gh-proxy.org/": {},
}

var releaseProxyOrder = []string{
	"https://gh-proxy.org/",
	"https://v4.gh-proxy.org/",
	"https://cdn.gh-proxy.org/",
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type downloadReporter func(status, message string, downloaded, total int64, failure string)

func runtimePlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

func fetchOfficialReleases(ctx context.Context, selectedProxy string) ([]Release, error) {
	var failures []string
	for _, url := range releaseListURLs(selectedProxy) {
		releases, err := fetchOfficialReleasesAt(ctx, url)
		if err == nil {
			return releases, nil
		}
		failures = append(failures, err.Error())
	}
	return nil, fmt.Errorf("获取 FRP 官方版本列表失败：%s", strings.Join(failures, "；"))
}

func fetchOfficialReleasesAt(ctx context.Context, url string) ([]Release, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	var source []githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&source); err != nil {
		return nil, fmt.Errorf("响应解析失败：%w", err)
	}
	return supportedReleases(source), nil
}

func releaseListURLs(selectedProxy string) []string {
	urls := []string{}
	add := func(proxy string) {
		url := githubReleasesURL
		if proxy != "" {
			url = proxy + githubReleasesURL
		}
		for _, item := range urls {
			if item == url {
				return
			}
		}
		urls = append(urls, url)
	}
	add(selectedProxy)
	add("")
	for _, proxy := range releaseProxyOrder {
		add(proxy)
	}
	return urls
}

func supportedReleases(source []githubRelease) []Release {
	releases := make([]Release, 0, len(source))
	for _, item := range source {
		if item.Draft || item.Prerelease {
			continue
		}
		asset, ok := findArchiveAsset(item)
		if !ok {
			continue
		}
		releases = append(releases, Release{Version: item.TagName, PublishedAt: item.PublishedAt.UTC().Format(time.RFC3339), AssetName: asset.Name, Size: asset.Size, Checksum: strings.TrimPrefix(asset.Digest, "sha256:"), AssetID: asset.ID})
	}
	return releases
}

func findArchiveAsset(release githubRelease) (githubAsset, bool) {
	version := strings.TrimPrefix(release.TagName, "v")
	prefix := "frp_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH
	for _, asset := range release.Assets {
		if asset.Name == prefix+".zip" || asset.Name == prefix+".tar.gz" {
			return asset, true
		}
	}
	return githubAsset{}, false
}

func findRelease(releases []Release, version string) (Release, bool) {
	for _, release := range releases {
		if release.Version == version {
			return release, true
		}
	}
	return Release{}, false
}

func downloadOfficialBinary(ctx context.Context, frpDir string, release Release, proxy string, report downloadReporter) (string, error) {
	checksum := release.Checksum
	if checksum == "" {
		report("queued", "正在获取官方 SHA-256 校验信息", 0, 0, "")
		var err error
		checksum, err = fetchChecksum(ctx, release)
		if err != nil {
			return "", err
		}
	}
	archivePath, err := downloadArchive(ctx, frpDir, release, checksum, proxy, report)
	if err != nil {
		return "", err
	}
	defer os.Remove(archivePath)
	report("extracting", "SHA-256 校验通过，正在解压 frpc", 0, 0, "")
	return extractClientBinary(archivePath, frpDir, release)
}

// checksumAssetNames 官方校验文件名候选（按优先级）。
// frp 从早期版本起就一直是 frp_sha256_checksums.txt（不带版本号），
// 下面保留一个旧名字兼底，避开上游再次改名时整条下载链路直接 404。
func checksumAssetCandidates(version string) []string {
	return []string{
		"frp_sha256_checksums.txt",
		"frp_" + version + "_sha256sums.txt",
	}
}

func fetchChecksum(ctx context.Context, release Release) (string, error) {
	version := strings.TrimPrefix(release.Version, "v")
	candidates := checksumAssetCandidates(version)
	var lastErr error
	for _, name := range candidates {
		data, err := fetchBytes(ctx, officialAssetURL(release.Version, name), 1<<20)
		if err != nil {
			lastErr = err
			continue
		}
		if sum, ok := findChecksumLine(data, release.AssetName); ok {
			return sum, nil
		}
		lastErr = fmt.Errorf("FRP 官方校验文件 %s 中未找到 %s", name, release.AssetName)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的官方校验文件")
	}
	return "", fmt.Errorf("下载 FRP 官方校验文件失败：%v（已尝试：%s）", lastErr, strings.Join(candidates, "、"))
}

// findChecksumLine 解析 sha256sum 格式（<hash>  <文件名>，文件名可能带 * 前缀）。
func findChecksumLine(data []byte, assetName string) (string, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == assetName {
			return fields[0], true
		}
	}
	return "", false
}

func downloadArchive(ctx context.Context, frpDir string, release Release, checksum, proxy string, report downloadReporter) (string, error) {
	report("downloading", "正在下载 FRP 官方安装包", 0, release.Size, "")
	data, err := fetchBytesWithProgress(ctx, releaseDownloadURL(release, proxy), maxArchiveSize, func(downloaded, total int64) {
		if total <= 0 {
			total = release.Size
		}
		report("downloading", "正在下载 FRP 官方安装包", downloaded, total, "")
	})
	if err != nil {
		return "", fmt.Errorf("下载 FRP 官方安装包失败：%w；请检查网络连接或设置 HTTPS_PROXY", err)
	}
	report("verifying", "安装包下载完成，正在校验 SHA-256", int64(len(data)), int64(len(data)), "")
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	if !strings.EqualFold(sum, checksum) {
		return "", fmt.Errorf("FRP 安装包 SHA-256 校验失败")
	}
	dir := filepath.Join(frpDir, "downloads")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, release.AssetName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func fetchBytes(ctx context.Context, url string, limit int64) ([]byte, error) {
	return fetchBytesWithProgress(ctx, url, limit, nil)
}

func fetchBytesWithProgress(ctx context.Context, url string, limit int64, progress func(int64, int64)) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Havline-FRP-Downloader")
	if strings.HasPrefix(url, "https://api.github.com/repos/fatedier/frp/releases/assets/") {
		request.Header.Set("Accept", "application/octet-stream")
		request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return readResponseBody(response.Body, response.ContentLength, limit, progress)
}

func readResponseBody(reader io.Reader, total, limit int64, progress func(int64, int64)) ([]byte, error) {
	var output bytes.Buffer
	buffer := make([]byte, 64*1024)
	var downloaded int64
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			downloaded += int64(count)
			if downloaded > limit {
				return nil, fmt.Errorf("下载内容超过大小限制")
			}
			_, _ = output.Write(buffer[:count])
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if err == io.EOF {
			return output.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func officialAssetURL(version, name string) string {
	return "https://github.com/fatedier/frp/releases/download/" + version + "/" + name
}

func releaseDownloadURL(release Release, proxy string) string {
	if proxy != "" {
		return proxy + officialAssetURL(release.Version, release.AssetName)
	}
	if release.AssetID > 0 {
		return fmt.Sprintf("https://api.github.com/repos/fatedier/frp/releases/assets/%d", release.AssetID)
	}
	return officialAssetURL(release.Version, release.AssetName)
}

func downloadProxyPath(frpDir string) string {
	return filepath.Join(frpDir, "download-proxy.json")
}

func readDownloadProxy(frpDir string) (string, error) {
	data, err := os.ReadFile(downloadProxyPath(frpDir))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var state struct {
		Proxy string `json:"proxy"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return "", fmt.Errorf("FRP 下载代理配置无效")
	}
	if !validDownloadProxy(state.Proxy) {
		return "", fmt.Errorf("FRP 下载代理配置无效")
	}
	return state.Proxy, nil
}

func writeDownloadProxy(frpDir, proxy string) error {
	if !validDownloadProxy(proxy) {
		return fmt.Errorf("不支持的 FRP 下载加速代理")
	}
	if err := os.MkdirAll(frpDir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(struct {
		Proxy string `json:"proxy"`
	}{Proxy: proxy})
	if err != nil {
		return err
	}
	return os.WriteFile(downloadProxyPath(frpDir), data, 0o600)
}

func validDownloadProxy(proxy string) bool {
	_, ok := downloadProxyOptions[proxy]
	return ok
}

func extractClientBinary(archivePath, frpDir string, release Release) (string, error) {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return "", err
	}
	binary, err := extractBinary(data, release.AssetName)
	if err != nil {
		return "", err
	}
	path := filepath.Join(frpDir, "bin", release.Version, clientBinaryName())
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, binary, 0o700); err != nil {
		return "", err
	}
	return path, nil
}

func extractBinary(data []byte, assetName string) ([]byte, error) {
	if strings.HasSuffix(assetName, ".zip") {
		return extractZipBinary(data)
	}
	return extractTarBinary(data)
}

func extractZipBinary(data []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("FRP ZIP 安装包无效：%w", err)
	}
	for _, file := range reader.File {
		if filepath.Base(file.Name) != clientBinaryName() || file.FileInfo().IsDir() {
			continue
		}
		return readZipFile(file)
	}
	return nil, fmt.Errorf("FRP 安装包中未找到 %s", clientBinaryName())
}

func readZipFile(file *zip.File) ([]byte, error) {
	if file.UncompressedSize64 > maxArchiveSize {
		return nil, fmt.Errorf("FRP 客户端文件超过大小限制")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return boundedRead(reader, maxArchiveSize)
}

func extractTarBinary(data []byte) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("FRP TAR.GZ 安装包无效：%w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != clientBinaryName() {
			continue
		}
		if header.Size > maxArchiveSize {
			return nil, fmt.Errorf("FRP 客户端文件超过大小限制")
		}
		return boundedRead(reader, maxArchiveSize)
	}
	return nil, fmt.Errorf("FRP 安装包中未找到 %s", clientBinaryName())
}

func boundedRead(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("FRP 客户端文件超过大小限制")
	}
	return data, nil
}

func clientBinaryName() string {
	if runtime.GOOS == "windows" {
		return "frpc.exe"
	}
	return "frpc"
}

func listBinaries(frpDir, active string) ([]Binary, error) {
	root := filepath.Join(frpDir, "bin")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []Binary{}, nil
	}
	if err != nil {
		return nil, err
	}
	binaries := make([]Binary, 0, len(entries))
	for _, entry := range entries {
		binary, ok := binaryFromEntry(root, entry, active)
		if ok {
			binaries = append(binaries, binary)
		}
	}
	sort.Slice(binaries, func(i, j int) bool { return binaries[i].Version > binaries[j].Version })
	return binaries, nil
}

func binaryFromEntry(root string, entry os.DirEntry, active string) (Binary, bool) {
	if !entry.IsDir() || !validVersion(entry.Name()) {
		return Binary{}, false
	}
	path := filepath.Join(root, entry.Name(), clientBinaryName())
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return Binary{}, false
	}
	return Binary{Version: entry.Name(), Path: path, Size: info.Size(), InstalledAt: info.ModTime().UTC().Format(time.RFC3339), Active: filepath.Clean(path) == filepath.Clean(active)}, true
}

func binaryPathForVersion(frpDir, version string) (string, error) {
	if !validVersion(version) {
		return "", fmt.Errorf("FRP 版本格式无效")
	}
	path := filepath.Join(frpDir, "bin", version, clientBinaryName())
	if !fileExists(path) {
		return "", fmt.Errorf("FRP %s 尚未下载", version)
	}
	return path, nil
}

func removeBinaryVersion(frpDir, version, binaryPath string) error {
	root, err := filepath.Abs(filepath.Join(frpDir, "bin"))
	if err != nil {
		return err
	}
	target, err := filepath.Abs(filepath.Dir(binaryPath))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || filepath.Clean(relative) != version {
		return fmt.Errorf("FRP 客户端目录无效")
	}
	return os.RemoveAll(target)
}

func validVersion(version string) bool {
	if version == "" || len(version) > 32 || strings.ContainsAny(version, `/\\`) {
		return false
	}
	for _, char := range version {
		if (char < '0' || char > '9') && char != '.' && char != 'v' && char != '-' {
			return false
		}
	}
	return true
}

func activeBinaryPath(frpDir string) string {
	return filepath.Join(frpDir, "bin", "active.json")
}

func readActiveBinary(frpDir string) (string, error) {
	data, err := os.ReadFile(activeBinaryPath(frpDir))
	if err != nil {
		return "", err
	}
	var state struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(data, &state); err != nil || versionFromBinaryPath(frpDir, state.Path) == "" || !fileExists(state.Path) {
		return "", fmt.Errorf("已启用的 FRP 客户端无效")
	}
	return state.Path, nil
}

func writeActiveBinary(frpDir, path string) error {
	if err := os.MkdirAll(filepath.Dir(activeBinaryPath(frpDir)), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(struct {
		Path string `json:"path"`
	}{Path: path})
	if err != nil {
		return err
	}
	return os.WriteFile(activeBinaryPath(frpDir), data, 0o600)
}

func versionFromBinaryPath(frpDir, path string) string {
	root := filepath.Join(frpDir, "bin")
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) != 2 || !validVersion(parts[0]) {
		return ""
	}
	return parts[0]
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
