#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"
if [[ "$(uname -s)" != Darwin ]]; then
    echo "Build the macOS release on a Mac." >&2
    exit 1
fi
resources="resources/darwin/media-control"
if [[ ! -x "$resources/bin/media-control" ]]; then
    if ! command -v cmake >/dev/null; then
        echo "CMake is required on the build Mac to compile the bundled media adapter." >&2
        exit 1
    fi

    work="$(mktemp -d)"
    trap 'rm -rf -- "$work"' EXIT

    # The tag and its submodule pin the CLI and native adapter shipped together.
    git -c advice.detachedHead=false clone --quiet --depth 1 --branch v0.7.7 --recurse-submodules --shallow-submodules \
        https://github.com/ungive/media-control.git "$work/source"
    cmake -S "$work/source" -B "$work/build" -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_OSX_DEPLOYMENT_TARGET=12.0
    cmake --build "$work/build" --config Release --parallel
    cmake --install "$work/build" --config Release --prefix "$work/media-control"
    {
        printf '%s\n\n' 'media-control — Copyright (c) 2025 Jonas van den Berg'
        cat "$work/source/mediaremote-adapter/LICENSE"
    } > "$work/media-control/LICENSE"

    mkdir -p resources/darwin
    mv "$work/media-control" "$resources"
fi

meter=resources/darwin/audio-level
if [[ ! -x "$meter" || native/macos/audio-level.m -nt "$meter" || native/macos/audio-level.plist -nt "$meter" ]]; then
    mkdir -p resources/darwin
    xcrun clang -fobjc-arc -O2 -arch arm64 -arch x86_64 -mmacosx-version-min=12.0 \
        -framework Foundation -framework CoreAudio \
        -Wl,-sectcreate,__TEXT,__info_plist,native/macos/audio-level.plist \
        native/macos/audio-level.m -o "$meter"
fi

if [[ "${1:-}" == --prepare-media ]]; then
    exit 0
fi
go tool mygo build -platform darwin/universal "$@"
