$ErrorActionPreference = 'Stop'

function Invoke-TimedJsonPost {
  param(
    [Parameter(Mandatory=$true)][string]$Url,
    [Parameter(Mandatory=$true)][hashtable]$Headers,
    [Parameter(Mandatory=$true)][hashtable]$Body,
    [int]$TimeoutSec = 15
  )
  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  try {
    $resp = Invoke-RestMethod -Method Post -Uri $Url -Headers $Headers -Body ($Body | ConvertTo-Json -Depth 10) -ContentType 'application/json' -TimeoutSec $TimeoutSec
    $sw.Stop()
    return @{ ok=$true; ms=$sw.ElapsedMilliseconds; data=$resp }
  } catch {
    $sw.Stop()
    return @{ ok=$false; ms=$sw.ElapsedMilliseconds; err=$_.Exception.Message }
  }
}

function Invoke-TimedGet {
  param(
    [Parameter(Mandatory=$true)][string]$Url,
    [hashtable]$Headers = @{},
    [int]$TimeoutSec = 5
  )
  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  try {
    $resp = Invoke-RestMethod -Method Get -Uri $Url -Headers $Headers -TimeoutSec $TimeoutSec
    $sw.Stop()
    return @{ ok=$true; ms=$sw.ElapsedMilliseconds; data=$resp }
  } catch {
    $sw.Stop()
    return @{ ok=$false; ms=$sw.ElapsedMilliseconds; err=$_.Exception.Message }
  }
}

$gatewayUrl = $env:CLAUDIA_GATEWAY_URL
if (-not $gatewayUrl) { $gatewayUrl = 'http://127.0.0.1:3000' } # avoid IPv6 localhost quirks
$gatewayUrl = $gatewayUrl.TrimEnd('/')
$gatewayToken = $env:CLAUDIA_GATEWAY_TOKEN
if (-not $gatewayToken) { $gatewayToken = 'claudia-loves-lynn' } # default used by orchestrator

$orchestratorUrl = 'https://127.0.0.1:11435' # avoid IPv6 localhost quirks

# Orchestrator uses a local self-signed cert; skip validation for this script only.
try {
  [System.Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }
} catch { }

Write-Host ''
Write-Host 'Claudia Routing Check'
Write-Host ('- Orchestrator: {0}' -f $orchestratorUrl)
Write-Host ('- Gateway:      {0}' -f $gatewayUrl)
Write-Host ''

$gwHeaders = @{ Authorization = ('Bearer {0}' -f $gatewayToken) }

$gwHealth = Invoke-TimedGet -Url ($gatewayUrl + '/health') -Headers $gwHeaders -TimeoutSec 3
$gwHealthLabel = 'FAIL'
if ($gwHealth.ok) { $gwHealthLabel = 'OK' }
Write-Host ('Gateway /health:  {0} ({1} ms)' -f $gwHealthLabel, $gwHealth.ms)
if (-not $gwHealth.ok) { Write-Host ('  {0}' -f $gwHealth.err) }

$gwStatus = Invoke-TimedGet -Url ($gatewayUrl + '/status') -Headers $gwHeaders -TimeoutSec 3
$gwStatusLabel = 'FAIL'
if ($gwStatus.ok) { $gwStatusLabel = 'OK' }
Write-Host ('Gateway /status:  {0} ({1} ms)' -f $gwStatusLabel, $gwStatus.ms)
if ($gwStatus.ok) {
  $vm = ($gwStatus.data.gateway.virtual_model)
  if ($vm) { Write-Host ('  virtual_model: {0}' -f $vm) }
} else {
  Write-Host ('  {0}' -f $gwStatus.err)
}

# Orchestrator config endpoint exists in the original app; it helps confirm you are hitting 11435.
$orchConfig = Invoke-TimedGet -Url ($orchestratorUrl + '/api/config') -TimeoutSec 3
$orchLabel = 'FAIL'
if ($orchConfig.ok) { $orchLabel = 'OK' }
Write-Host ('Orchestrator /api/config: {0} ({1} ms)' -f $orchLabel, $orchConfig.ms)
if (-not $orchConfig.ok) { Write-Host ('  {0}' -f $orchConfig.err) }

# Gateway chat probe: tiny non-stream request. This validates whether the gateway can answer quickly at all.
$model = $null
if ($gwStatus.ok -and $gwStatus.data.gateway -and $gwStatus.data.gateway.virtual_model) {
  $model = [string]$gwStatus.data.gateway.virtual_model
}
if (-not $model) { $model = $env:CLAUDIA_GATEWAY_VIRTUAL_MODEL }
if (-not $model) { $model = 'Claudia-0.2.0' }

$probe = Invoke-TimedJsonPost `
  -Url ($gatewayUrl + '/v1/chat/completions') `
  -Headers (@{ Authorization=('Bearer {0}' -f $gatewayToken) }) `
  -Body @{
    model = $model
    stream = $false
    messages = @(
      @{ role='system'; content='You are a health check. Reply with exactly: ok' },
      @{ role='user'; content='ping' }
    )
    max_tokens = 5
  } `
  -TimeoutSec 20

$probeLabel = 'FAIL'
if ($probe.ok) { $probeLabel = 'OK' }
Write-Host ('Gateway chat probe: {0} ({1} ms)' -f $probeLabel, $probe.ms)
if ($probe.ok) {
  try {
    $txt = $probe.data.choices[0].message.content
    if ($txt) { Write-Host ('  reply: {0}' -f (($txt -replace '\\s+',' ').Trim())) }
  } catch { }
} else {
  Write-Host ('  {0}' -f $probe.err)
}

Write-Host ''
Write-Host 'Notes:'
Write-Host '- If the gateway probe is slow or fails, the orchestrator UI may hang while it tries the gateway before falling back.'
Write-Host '- If the gateway probe is fast but the UI still hangs, the bottleneck is likely inside the orchestrator (tooling/search/ollama) or the browser connection.'
Write-Host ''
