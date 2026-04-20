param(
    [string]$MediaDir = $env:ATLAS_MEDIA_DIR,
    [string]$DatabasePath = (Join-Path (Get-Location) "atlas-smoke.db"),
    [string]$ListenAddress = "127.0.0.1:18083"
)

$ErrorActionPreference = "Stop"

if (-not $MediaDir) {
    throw "Set ATLAS_MEDIA_DIR or pass -MediaDir before running the smoke test."
}

$root = Split-Path -Parent $PSScriptRoot
$dbFullPath = [System.IO.Path]::GetFullPath($DatabasePath)
Remove-Item -LiteralPath $dbFullPath -ErrorAction SilentlyContinue

$env:GOCACHE = Join-Path $root ".gocache"
$env:ATLAS_MEDIA_DIR = $MediaDir
$env:ATLAS_DATABASE_PATH = $dbFullPath
$env:ATLAS_LISTEN_ADDR = $ListenAddress

$server = Start-Process go -ArgumentList "run", "./cmd/atlas-audio-engine" -WorkingDirectory $root -PassThru

try {
    $baseUrl = "http://$ListenAddress"
    $health = $null
    for ($i = 0; $i -lt 30; $i++) {
        Start-Sleep -Seconds 1
        try {
            $health = Invoke-RestMethod -Uri "$baseUrl/health" -TimeoutSec 2
            break
        } catch {}
    }
    if (-not $health) {
        throw "server did not become healthy in time"
    }

    $channels = @(Invoke-RestMethod -Uri "$baseUrl/channels" -TimeoutSec 10)
    if ($channels.Count -eq 0) {
        throw "no channels returned"
    }

    $channelId = $channels[0].id
    $library = @(Invoke-RestMethod -Uri "$baseUrl/channels/$channelId/library" -TimeoutSec 10)
    if ($library.Count -eq 0) {
        throw "library is empty"
    }

    $playlist = @(Invoke-RestMethod -Uri "$baseUrl/channels/$channelId/playlist" -TimeoutSec 10)
    if ($playlist.Count -eq 0) {
        throw "playlist is empty"
    }

    $firstTrackId = if ($playlist[0].track_id) { $playlist[0].track_id } else { $library[0].id }
    $queueBody = @{ track_id = $firstTrackId } | ConvertTo-Json -Compress
    $queueItem = Invoke-RestMethod -Method Post -Uri "$baseUrl/channels/$channelId/queue" -ContentType "application/json" -Body $queueBody -TimeoutSec 10
    $queue = @(Invoke-RestMethod -Uri "$baseUrl/channels/$channelId/queue" -TimeoutSec 10)
    if ($queue.Count -eq 0) {
        throw "queue add did not persist"
    }

    Invoke-RestMethod -Method Delete -Uri "$baseUrl/channels/$channelId/queue/$($queueItem.id)" -TimeoutSec 10 | Out-Null

    if ($library.Count -gt 1) {
        $playlistBody = @{ track_ids = @($library[1].id, $library[0].id) } | ConvertTo-Json -Compress
        $edited = @(Invoke-RestMethod -Method Put -Uri "$baseUrl/channels/$channelId/playlist" -ContentType "application/json" -Body $playlistBody -TimeoutSec 10)
        if ($edited[0].track_id -ne $library[1].id) {
            throw "playlist edit did not apply"
        }
    }

    $state = Invoke-RestMethod -Uri "$baseUrl/channels/$channelId/state" -TimeoutSec 10
    $skip = Invoke-RestMethod -Method Post -Uri "$baseUrl/channels/$channelId/skip" -TimeoutSec 10

    [PSCustomObject]@{
        Status = "ok"
        ChannelID = $channelId
        LibraryTracks = $library.Count
        PlaylistTracks = $playlist.Count
        NowPlaying = $state.now_playing.title
        SkippedTo = $skip.title
    } | ConvertTo-Json -Compress
} finally {
    if ($server) {
        Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue
    }
    Remove-Item -LiteralPath $dbFullPath -ErrorAction SilentlyContinue
}
