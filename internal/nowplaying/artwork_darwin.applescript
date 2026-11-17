on run argv
    if application id "com.apple.Music" is not running then return missing value
    tell application id "com.apple.Music"
        set selectedTrack to current track
        if persistent ID of selectedTrack is not item 1 of argv then return missing value
        if (count of artworks of selectedTrack) is 0 then return missing value
        return raw data of artwork 1 of selectedTrack
    end tell
end run
