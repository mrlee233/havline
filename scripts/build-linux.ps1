<#
.SYNOPSIS
  构建 Havline 的 Linux 安装包（tar.gz：二进制 + migrations + systemd 安装脚本）。

.DESCRIPTION
  与 Dockerfile 同源的三步：
    1) 前端构建（VITE_APP_VERSION 取根目录 VERSION，dist 会被 //go:embed 进二进制）
    2) 交叉编译 havline-agent 双架构到 internal/frp/agentbin（主程序构建时内嵌）
    3) CGO_ENABLED=0 交叉编译主程序并打包
  依赖是纯 Go 的（modernc.org/sqlite），所以 Windows 上就能产出 Linux 包，不需要 Docker/WSL。

.EXAMPLE
  ./scripts/build-linux.ps1                  # 双架构
  ./scripts/build-linux.ps1 -Arch amd64      # 只出 amd64
  ./scripts/build-linux.ps1 -SkipWeb         # 前端已构建过，跳过 npm
#>
param(
  [ValidateSet('amd64', 'arm64', 'both')] [string]$Arch = 'both',
  [string]$OutDir = 'dist',
  [switch]$SkipWeb,
  [switch]$SkipAgent
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$version = (Get-Content (Join-Path $root 'VERSION') -Raw).Trim()
if (-not $version) { throw 'VERSION 文件为空，无法确定版本号' }
Write-Host "==> Havline $version" -ForegroundColor Cyan

# ---- 1) 前端：dist 缺失会让主程序编译失败（//go:embed all:web/dist）
if (-not $SkipWeb) {
  Push-Location (Join-Path $root 'web')
  try {
    if (-not (Test-Path 'node_modules')) {
      Write-Host '==> npm ci（首次构建，需要网络）'
      npm ci
      if ($LASTEXITCODE -ne 0) { throw "npm ci 失败（exit $LASTEXITCODE）" }
    }
    $env:VITE_APP_VERSION = $version
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "前端构建失败（exit $LASTEXITCODE）" }
  } finally {
    Pop-Location
    Remove-Item Env:VITE_APP_VERSION -ErrorAction SilentlyContinue
  }
}
if (-not (Test-Path (Join-Path $root 'cmd/havline/web/dist/index.html'))) {
  throw '缺少 cmd/havline/web/dist/index.html：请先构建前端（或去掉 -SkipWeb）'
}

# ---- 2) havline-agent 双架构：主程序会内嵌这两个文件，必须在构建主程序前就位
if (-not $SkipAgent) {
  Write-Host '==> 交叉编译 havline-agent（amd64/arm64）'
  $env:CGO_ENABLED = '0'
  $env:GOOS = 'linux'
  foreach ($a in @('amd64', 'arm64')) {
    $env:GOARCH = $a
    go build -trimpath -o "internal/frp/agentbin/havline-agent-linux-$a" ./cmd/havline-agent
    if ($LASTEXITCODE -ne 0) { throw "havline-agent($a) 构建失败（exit $LASTEXITCODE）" }
  }
}

# ---- 3) 主程序 + 打包
$archs = if ($Arch -eq 'both') { @('amd64', 'arm64') } else { @($Arch) }
$outPath = Join-Path $root $OutDir
New-Item -ItemType Directory -Force -Path $outPath | Out-Null

try {
  foreach ($a in $archs) {
    $name = "havline-$version-linux-$a"
    $stage = Join-Path $outPath $name
    if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
    New-Item -ItemType Directory -Force -Path (Join-Path $stage 'migrations') | Out-Null

    Write-Host "==> 交叉编译 havline（linux/$a）"
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'linux'
    $env:GOARCH = $a
    go build -trimpath `
      -ldflags "-s -w -X github.com/havline/havline/internal/version.Version=$version" `
      -o (Join-Path $stage 'havline') ./cmd/havline
    if ($LASTEXITCODE -ne 0) { throw "havline($a) 构建失败（exit $LASTEXITCODE）" }

    # 包内容与 Dockerfile 的运行时 stage 对齐：migrations 必须与二进制同级（程序按工作目录找）
    Copy-Item (Join-Path $root 'migrations/*.sql') (Join-Path $stage 'migrations') -Force
    Copy-Item (Join-Path $root 'scripts/havline-install.sh') (Join-Path $stage 'install.sh') -Force
    Copy-Item (Join-Path $root 'VERSION') $stage -Force
    Copy-Item (Join-Path $root 'LICENSE') $stage -Force
    if (Test-Path (Join-Path $root 'web/assets/image')) {
      New-Item -ItemType Directory -Force -Path (Join-Path $stage 'web/assets') | Out-Null
      Copy-Item -Recurse -Force (Join-Path $root 'web/assets/image') (Join-Path $stage 'web/assets')
    }

    $tarball = Join-Path $outPath "$name.tar.gz"
    if (Test-Path $tarball) { Remove-Item -Force $tarball }
    tar -czf $tarball -C $outPath $name
    if ($LASTEXITCODE -ne 0) { throw "打包失败（exit $LASTEXITCODE）" }

    $hash = (Get-FileHash $tarball -Algorithm SHA256).Hash.ToLower()
    "$hash  $name.tar.gz" | Out-File -Encoding ascii "$tarball.sha256"
    $sizeMB = [math]::Round((Get-Item $tarball).Length / 1MB, 2)
    Write-Host ("    {0}.tar.gz  {1} MB  sha256 {2}" -f $name, $sizeMB, $hash.Substring(0, 16)) -ForegroundColor Green
  }
} finally {
  # 交叉编译用的环境变量必须复位，否则后续 go vet/test 会继续按 linux 目标跑
  Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

Write-Host "==> 产物在 $outPath" -ForegroundColor Cyan
