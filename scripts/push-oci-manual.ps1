<#
Manual OCI layout pusher for registries whose token realm is misconfigured.

This script is an emergency fallback. It reads Docker's OCI layout output,
obtains a Bearer token over HTTPS from the Git credential store, and uploads
the blobs and image manifest directly through the Registry HTTP API.
#>
param(
  [Parameter(Mandatory = $true)][string]$LayoutDir,
  [Parameter(Mandatory = $true)][string]$Platform,
  [Parameter(Mandatory = $true)][string]$Tag,
  [string]$Registry = "jayhub.top",
  [string]$Repository = "mrlee/havline",
  [string]$CredentialHost = "jayhub.top"
)

$ErrorActionPreference = "Stop"
$script:Registry = $Registry
$script:Repository = $Repository

function Get-RegistryBasicAuth {
  $cred = "protocol=https`nhost=$CredentialHost`n`n" | git credential fill 2>$null
  $values = @{}
  $cred -split "`n" | ForEach-Object {
    if ($_ -match '^(.*?)=(.*)$') { $values[$matches[1]] = $matches[2] }
  }
  if (-not $values.username -or -not $values.password) {
    throw "No Git credential found for $CredentialHost"
  }
  $raw = $values.username + ":" + $values.password
  return "Basic " + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($raw))
}

function Get-BearerToken([string]$Scope) {
  $basic = Get-RegistryBasicAuth
  $url = "https://$Registry/v2/token?service=container_registry&scope=$Scope"
  $response = Invoke-RestMethod -Uri $url -Headers @{ Authorization = $basic }
  if (-not $response.token) { throw "Registry did not return a token" }
  return $response.token
}

function Get-BlobPath([string]$LayoutRoot, [string]$Digest) {
  return Join-Path $LayoutRoot ("blobs\sha256\" + ($Digest -replace '^sha256:', ''))
}

function Read-JsonBlob([string]$LayoutRoot, [string]$Digest) {
  $path = Get-BlobPath $LayoutRoot $Digest
  return Get-Content $path -Raw | ConvertFrom-Json
}

function Find-PlatformManifest($Index, [string]$Arch, [string]$Os) {
  foreach ($entry in $Index.manifests) {
    if ($entry.platform.architecture -eq $Arch -and $entry.platform.os -eq $Os) {
      return $entry
    }
  }
  return $null
}

function Resolve-ImageManifest([string]$LayoutRoot, [string]$Arch, [string]$Os) {
  $index = Get-Content (Join-Path $LayoutRoot "index.json") -Raw | ConvertFrom-Json
  foreach ($entry in $index.manifests) {
    $document = Read-JsonBlob $LayoutRoot $entry.digest
    if ($document.mediaType -eq "application/vnd.oci.image.manifest.v1+json") {
      return @{ Digest = $entry.digest; Document = $document }
    }
    $match = Find-PlatformManifest $document $Arch $Os
    if ($match) {
      return @{ Digest = $match.digest; Document = (Read-JsonBlob $LayoutRoot $match.digest) }
    }
  }
  throw "No image manifest found for $Os/$Arch"
}

function Test-BlobExists([string]$Digest, [string]$Token) {
  $url = "https://$Registry/v2/$Repository/blobs/$Digest"
  $response = Invoke-WebRequest -Uri $url -Method Head -Headers @{ Authorization = "Bearer $Token" } -SkipHttpErrorCheck
  return $response.StatusCode -eq 200
}

function Push-Blob([string]$LayoutRoot, [string]$Digest, [string]$Token) {
  if (Test-BlobExists $Digest $Token) {
    Write-Output "Blob already exists: $Digest"
    return
  }
  $postUrl = "https://$Registry/v2/$Repository/blobs/uploads/"
  $post = Invoke-WebRequest -Uri $postUrl -Method Post -Headers @{ Authorization = "Bearer $Token" } -SkipHttpErrorCheck
  if ($post.StatusCode -ne 202) { throw "Create upload session failed: HTTP $($post.StatusCode)" }
  $location = $post.Headers["Location"]
  if ($location -is [array]) { $location = $location[0] }
  if ($location -notmatch '^https?://') { $location = "https://$Registry$location" }
  $separator = "?"
  if ($location -match '\?') { $separator = "&" }
  $putUrl = $location + $separator + "digest=" + $Digest
  $path = Get-BlobPath $LayoutRoot $Digest
  $curlArgs = @(
    "-sS", "-o", "NUL", "-w", "%{http_code}", "-X", "PUT",
    "-H", "Authorization: Bearer $Token",
    "-H", "Content-Type: application/octet-stream",
    "--data-binary", ("@" + $path), $putUrl
  )
  $status = (& curl.exe @curlArgs).Trim()
  if ($status -ne "201") { throw "Upload blob failed: $Digest -> HTTP $status" }
  Write-Output "Uploaded blob: $Digest"
}

function Push-Manifest([string]$TagValue, [string]$MediaType, [string]$Json, [string]$Token) {
  $url = "https://$Registry/v2/$Repository/manifests/$TagValue"
  $response = Invoke-WebRequest -Uri $url -Method Put -Headers @{ Authorization = "Bearer $Token" } -ContentType $MediaType -Body ([Text.Encoding]::UTF8.GetBytes($Json)) -SkipHttpErrorCheck
  if ($response.StatusCode -ne 201) {
    throw "Push manifest failed: HTTP $($response.StatusCode) $($response.Content)"
  }
  return $response.Headers["Docker-Content-Digest"]
}

$parts = $Platform -split "/"
if ($parts.Count -ne 2) { throw "Platform must look like linux/amd64" }
$layoutRoot = (Resolve-Path $LayoutDir).Path
$manifest = Resolve-ImageManifest $layoutRoot $parts[1] $parts[0]
$allBlobs = @($manifest.Document.config.digest) + @($manifest.Document.layers | ForEach-Object { $_.digest })
$token = Get-BearerToken "repository:$Repository`:push,pull"
foreach ($blob in $allBlobs) { Push-Blob $layoutRoot $blob $token }
$manifestJson = Get-Content (Get-BlobPath $layoutRoot $manifest.Digest) -Raw
$digest = Push-Manifest $Tag $manifest.Document.mediaType $manifestJson $token
Write-Output "Manifest: $digest"
Write-Output ("Tag: {0}/{1}:{2}" -f $Registry, $Repository, $Tag)
