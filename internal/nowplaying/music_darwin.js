function run(args) {
    const music = Application("com.apple.Music");
    const action = args[0];
    if (!music.running()) {
        if (action) throw new Error("Music has quit");
        return null;
    }

    switch (action) {
        case "previous-track": music.previousTrack(); return;
        case "next-track": music.nextTrack(); return;
        case "toggle-play-pause": music.playpause(); return;
        case "seek": music.playerPosition = Number(args[1]); return;
    }

    const state = music.playerState();
    if (state !== "playing" && state !== "paused") return null;

    const track = music.currentTrack;
    return JSON.stringify({
        id: track.persistentID(),
        title: track.name(),
        artist: track.artist(),
        album: track.album(),
        duration: track.duration(),
        position: music.playerPosition(),
        playing: state === "playing"
    });
}
