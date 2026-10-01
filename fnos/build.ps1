param(
  [string]$FnpackPath = ""
)

$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$project = Join-Path $root "fnos\havline"
$version = (Get-Content -Raw (Join-Path $root "VERSION")).Trim()

if (-not (Test-Path -LiteralPath $project)) {
  throw "找不到飞牛应用工程：$project"
}

if (-not $FnpackPath) {
  $localTool = Join-Path (Split-Path $root -Parent) "fnos\fnpack.exe"
  $FnpackPath = if (Test-Path -LiteralPath $localTool) { $localTool } else { "fnpack" }
}

$stamp = Get-Date -Format "yyyyMMddHHmmss"
$stage = Join-Path $PSScriptRoot ".build\havline-$stamp"
New-Item -ItemType Directory -Force $stage | Out-Null
robocopy $project $stage /E /XF .DS_Store /NFL /NDL /NJH /NJS /NP | Out-Null
if ($LASTEXITCODE -ge 8) {
  throw "复制飞牛应用工程失败，robocopy 退出码：$LASTEXITCODE"
}

Push-Location $PSScriptRoot
try {
  & $FnpackPath build -d $stage
  if ($LASTEXITCODE -ne 0) {
    throw "fnpack build 失败，退出码：$LASTEXITCODE"
  }
  $output = Join-Path $PSScriptRoot "havline.fpk"
  $dist = Join-Path $PSScriptRoot "dist"
  New-Item -ItemType Directory -Force $dist | Out-Null
  Copy-Item -LiteralPath $output -Destination (Join-Path $dist "havline-$version.fpk") -Force
  Write-Output "FPK: $(Join-Path $dist "havline-$version.fpk")"
}
finally {
  Pop-Location
}
