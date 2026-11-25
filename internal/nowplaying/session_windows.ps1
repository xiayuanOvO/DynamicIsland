$ErrorActionPreference = 'Stop'
[Console]::InputEncoding = [System.Text.UTF8Encoding]::new($false)
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$managerType = [Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager, Windows.Media.Control, ContentType=WindowsRuntime]
$mediaType = [Windows.Media.Control.GlobalSystemMediaTransportControlsSessionMediaProperties, Windows.Media.Control, ContentType=WindowsRuntime]
$streamType = [Windows.Storage.Streams.IRandomAccessStreamWithContentType, Windows.Storage.Streams, ContentType=WindowsRuntime]
$randomAccessType = [Windows.Storage.Streams.IRandomAccessStream, Windows.Storage.Streams, ContentType=WindowsRuntime]
$asStream = [System.IO.WindowsRuntimeStreamExtensions].GetMethod('AsStream', [type[]]@($randomAccessType))
$asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.IsGenericMethodDefinition -and
    $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
} | Select-Object -First 1

function Wait-Result($operation, [type]$resultType) {
    $task = $asTask.MakeGenericMethod($resultType).Invoke($null, @($operation))
    return $task.GetAwaiter().GetResult()
}

function Read-Artwork($reference) {
    try {
        $stream = Wait-Result ($reference.OpenReadAsync()) $streamType
        # Windows PowerShell sees the returned WinRT interface as a COM object.
        $inputStream = $asStream.Invoke($null, @($stream))
        try {
            $length = $inputStream.Length
            if ($length -gt 0 -and $length -le 8MB) {
                $reader = [IO.BinaryReader]::new($inputStream)
                $bytes = $reader.ReadBytes([int]$length)
                if ($bytes.Length -eq $length) { return [Convert]::ToBase64String($bytes) }
            }
        } finally { $inputStream.Dispose() }
    } catch {
        return ''
    }
    return ''
}

$manager = Wait-Result ($managerType::RequestAsync()) $managerType

function Handle-Read([bool]$withArtwork) {
    $session = $manager.GetCurrentSession()
    if ($null -eq $session) {
        return 'null'
    }
    $info = $session.GetPlaybackInfo()
    $controls = $info.Controls
    $playing = $info.PlaybackStatus -eq 'Playing'
    $media = Wait-Result ($session.TryGetMediaPropertiesAsync()) $mediaType
    if ($null -eq $media -or -not $media.Title) {
        return 'null'
    }
    $timeline = $session.GetTimelineProperties()
    $position = ($timeline.Position - $timeline.StartTime).TotalSeconds
    $duration = ($timeline.EndTime - $timeline.StartTime).TotalSeconds
    $rate = if ($null -eq $info.PlaybackRate) { 1 } else { $info.PlaybackRate }
    if ($playing -and $timeline.LastUpdatedTime.Year -gt 1900) {
        $position += [Math]::Max(0, ([DateTimeOffset]::UtcNow - $timeline.LastUpdatedTime).TotalSeconds) * $rate
    }
    $artwork = ''
    if ($withArtwork -and $null -ne $media.Thumbnail) {
        $artwork = Read-Artwork $media.Thumbnail
    }
    return (@{
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
    } | ConvertTo-Json -Compress)
}

function Handle-Control($action, $source, [long]$positionTicks) {
    $session = $manager.GetCurrentSession()
    if ($null -eq $session -or $session.SourceAppUserModelId -ne $source) {
        return '{"ok":false}'
    }
    $info = $session.GetPlaybackInfo()
    $controls = $info.Controls
    $playing = $info.PlaybackStatus -eq 'Playing'
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
            $session.TryChangePlaybackPositionAsync($start + $positionTicks)
        }
    }
    if ($null -eq $operation -or -not (Wait-Result $operation ([bool]))) {
        return '{"ok":false}'
    }
    return '{"ok":true}'
}

while ($null -ne ($line = [Console]::In.ReadLine())) {
    if (-not $line) { continue }
    try {
        $req = $line | ConvertFrom-Json
        switch ($req.op) {
            'ping' { [Console]::Out.WriteLine('{"ok":true}') }
            'read' { [Console]::Out.WriteLine((Handle-Read ([bool]$req.artwork))) }
            'control' { [Console]::Out.WriteLine((Handle-Control $req.action $req.source ([long]$req.position))) }
            default { [Console]::Out.WriteLine('{"error":"unknown op"}') }
        }
    } catch {
        [Console]::Out.WriteLine('{"error":"request failed"}')
    }
    [Console]::Out.Flush()
}
