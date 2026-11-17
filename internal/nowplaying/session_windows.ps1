$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$managerType = [Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager, Windows.Media.Control, ContentType=WindowsRuntime]
$mediaType = [Windows.Media.Control.GlobalSystemMediaTransportControlsSessionMediaProperties, Windows.Media.Control, ContentType=WindowsRuntime]
$streamType = [Windows.Storage.Streams.IRandomAccessStreamWithContentType, Windows.Storage.Streams, ContentType=WindowsRuntime]
$readerType = [Windows.Storage.Streams.DataReader, Windows.Storage.Streams, ContentType=WindowsRuntime]
$asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.IsGenericMethodDefinition -and
    $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
} | Select-Object -First 1

function Wait-Result($operation, [type]$resultType) {
    $task = $asTask.MakeGenericMethod($resultType).Invoke($null, @($operation))
    return $task.GetAwaiter().GetResult()
}

$manager = Wait-Result ($managerType::RequestAsync()) $managerType
$session = $manager.GetCurrentSession()
$action = $env:DYNAMICISLAND_MEDIA_ACTION
if ($null -eq $session) {
    if ($action) { exit 1 }
    'null'
    exit
}

$info = $session.GetPlaybackInfo()
$controls = $info.Controls
$playing = $info.PlaybackStatus -eq 'Playing'

if ($action) {
    if ($session.SourceAppUserModelId -ne $env:DYNAMICISLAND_MEDIA_SOURCE) { exit 1 }
    $operation = switch ($action) {
        'previous-track' { $session.TrySkipPreviousAsync() }
        'next-track' { $session.TrySkipNextAsync() }
        'toggle-play-pause' {
            if ($controls.IsPlayPauseToggleEnabled) { $session.TryTogglePlayPauseAsync() }
            elseif ($playing) { $session.TryPauseAsync() }
            else { $session.TryPlayAsync() }
        }
        'seek' {
            $start = $session.GetTimelineProperties().StartTime.Ticks
            $session.TryChangePlaybackPositionAsync($start + [long]$env:DYNAMICISLAND_MEDIA_POSITION)
        }
    }
    if ($null -eq $operation -or -not (Wait-Result $operation ([bool]))) { exit 1 }
    exit
}

$media = Wait-Result ($session.TryGetMediaPropertiesAsync()) $mediaType
if (-not $media.Title) {
    'null'
    exit
}

$timeline = $session.GetTimelineProperties()
$position = ($timeline.Position - $timeline.StartTime).TotalSeconds
$duration = ($timeline.EndTime - $timeline.StartTime).TotalSeconds
$rate = if ($null -eq $info.PlaybackRate) { 1 } else { $info.PlaybackRate }
if ($playing -and $timeline.LastUpdatedTime.Year -gt 1900) {
    $position += [Math]::Max(0, ([DateTimeOffset]::UtcNow - $timeline.LastUpdatedTime).TotalSeconds) * $rate
}

$artwork = ''
if ($env:DYNAMICISLAND_MEDIA_ARTWORK -eq 'true' -and $null -ne $media.Thumbnail) {
    try {
        $stream = Wait-Result ($media.Thumbnail.OpenReadAsync()) $streamType
        try {
            if ($stream.Size -gt 0 -and $stream.Size -le 8MB) {
                $reader = $readerType::new($stream.GetInputStreamAt(0))
                try {
                    $size = Wait-Result ($reader.LoadAsync([uint32]$stream.Size)) ([uint32])
                    $bytes = [byte[]]::new($size)
                    $reader.ReadBytes($bytes)
                    $artwork = [Convert]::ToBase64String($bytes)
                } finally { $reader.Dispose() }
            }
        } finally { $stream.Dispose() }
    } catch {
        $artwork = ''
    }
}

@{
    Source = $session.SourceAppUserModelId
    Title = $media.Title
    Artist = $media.Artist
    Album = $media.AlbumTitle
    Artwork = $artwork
    Playing = $playing
    Previous = $controls.IsPreviousEnabled
    Next = $controls.IsNextEnabled
    Toggle = $controls.IsPlayPauseToggleEnabled -or ($playing -and $controls.IsPauseEnabled) -or (-not $playing -and $controls.IsPlayEnabled)
    Seek = $controls.IsPlaybackPositionEnabled
    Position = $position
    Duration = $duration
    Rate = $rate
} | ConvertTo-Json -Compress
