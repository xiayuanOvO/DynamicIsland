on run argv
    if application id "com.apple.Music" is not running then return missing value
    tell application id "com.apple.Music"
        set selectedTrack to current track
        set expectedID to item 1 of argv
        if expectedID is not "" then
            if persistent ID of selectedTrack is not expectedID then return missing value
        else
            if name of selectedTrack is not item 2 of argv then return missing value
            if artist of selectedTrack is not item 3 of argv then return missing value
            if album of selectedTrack is not item 4 of argv then return missing value
        end if
        if (count of artworks of selectedTrack) is 0 then return missing value
        return raw data of artwork 1 of selectedTrack
    end tell
end run
