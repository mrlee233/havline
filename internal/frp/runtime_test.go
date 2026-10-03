package frp

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSupportedReleasesUsesOfficialDigest(t *testing.T) {
	version := "v0.99.0"
	extension := ".tar.gz"
	if runtime.GOOS == "windows" {
		extension = ".zip"
	}
	assetName := "frp_0.99.0_" + runtime.GOOS + "_" + runtime.GOARCH + extension
	releases := supportedReleases([]githubRelease{{
		TagName:     version,
		Assets:      []githubAsset{{ID: 123, Name: assetName, Size: 42, Digest: "sha256:abc123"}},
		PublishedAt: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC),
	}})
	if len(releases) != 1 || releases[0].Checksum != "abc123" || releases[0].AssetID != 123 {
		t.Fatalf("未读取官方资产摘要：%+v", releases)
	}
}

func TestExtractZipBinary(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("frp/" + clientBinaryName())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("client")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := extractZipBinary(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "client" {
		t.Fatalf("提取内容错误：%q", data)
	}
}

func TestValidDownloadProxy(t *testing.T) {
	for _, proxy := range []string{"", "https://gh-proxy.org/", "https://v4.gh-proxy.org/", "https://cdn.gh-proxy.org/"} {
		if !validDownloadProxy(proxy) {
			t.Fatalf("合法下载代理被拒绝：%s", proxy)
		}
	}
	if validDownloadProxy("https://untrusted.example/") {
		t.Fatal("非白名单下载代理不应被接受")
	}
}

func TestReleaseListURLsPrioritizesSelectedProxy(t *testing.T) {
	urls := releaseListURLs("https://v4.gh-proxy.org/")
	if urls[0] != "https://v4.gh-proxy.org/"+githubReleasesURL {
		t.Fatalf("未优先使用已选代理：%v", urls)
	}
	if urls[1] != githubReleasesURL {
		t.Fatalf("直连地址缺失：%v", urls)
	}
}

func TestRemoveBinaryVersion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bin", "v0.71.0", clientBinaryName())
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("client"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := removeBinaryVersion(root, "v0.71.0", path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("版本目录未删除：%v", err)
	}
}
