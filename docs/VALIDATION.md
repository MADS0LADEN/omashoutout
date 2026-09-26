# Validation

## Environment

Local validation uses Arch Linux, KDE Plasma, PipeWire 1.6.9 through its PulseAudio interface, and a Chromecast Audio named Lars Kontor. All hardware commands are constrained to a maximum receiver volume of 5%; tests use 1%, with -40 dB PCM attenuation.

## Verified so far

- Go builds; unit tests, race tests, and `go vet` pass.
- Direct IPv4 mDNS discovery identifies the intended receiver without a running Avahi daemon.
- The installed systemd user service starts and exposes the owned Shoutout sink and its monitor through the desktop audio server.
- The pre-existing default desktop output is preserved. The user confirmed that Shoutout appears in the KDE output selector.
- The receiver requests the continuous MP3 stream and reports PLAYING, with confirmed 1% receiver volume and mute status.
- Browser automation opens settings, discovers devices, exercises mute/unmute, and finds no JavaScript exceptions.
- A quiet three-second signal routed specifically to the Shoutout sink produced nonzero attenuated PCM while the receiver reported PLAYING, unmuted, at 1%. This validates the software path, not acoustic output.
- Restarting the installed service recreates the sink, reconnects, and returns to muted playback.
- Linux amd64 and arm64 builds succeeded locally.
- `govulncheck` reported no known vulnerabilities. The installed desktop entry and systemd unit pass their validators.
- Cast framing rejects malformed and oversized frames. Volume checks reject values above 5%, invalid numbers, and receiver reports outside the requested limit.
- Slow HTTP subscribers are disconnected instead of allowing an unbounded backlog.

## Limits

Audible output and end-to-end latency cannot be established from Cast's PLAYING status alone. Acoustic measurement and subjective listening are separate checks. UI presets currently adjust MP3 bitrate and the host queue; alternative codecs, real-time transports, automatic video synchronization, native distro packages, and automatic updates are not implemented.

The settings page is browser-based; the desktop entry opens it using the user's browser. KDE uses the regular audio-server sink, with no custom Plasma plugin. Fresh distro installation, speaker groups, hostile-network testing, forced network/audio-server recovery, and long-duration drift tests remain separate validation tasks.

## Hosted CI

The workflow is committed, but GitHub refused to start the initial jobs because of an account billing/spending-limit restriction. No workflow steps ran and no hosted build artifacts were produced. Local verification above is independent of that restriction.
