# Shoutout

A Linux virtual audio output that plays desktop audio through a Google Cast audio device. Select **Shoutout** in KDE's audio output menu, then choose the destination, volume, and playback preset in its settings.

**Status:** an initial working implementation. Continuous MP3 streaming is implemented; latency has not been optimized. Interactive, Video, Music, and Custom presets adjust encoding and host buffering, but do not guarantee game or video synchronization.

## Install

Requires Linux, PipeWire with its PulseAudio compatibility service (or a compatible PulseAudio server), `pactl`, `parec`, FFmpeg with `libmp3lame`, and a systemd user session.

On Arch, the runtime packages are `pipewire-pulse`, `libpulse`, and `ffmpeg`. On Debian/Ubuntu, the tools are provided by `pulseaudio-utils` and `ffmpeg`, alongside the desktop's audio server. Installation on those distributions still needs independent validation.

Build with the Go version declared in `go.mod` or newer:

```sh
make build
./bin/shoutout doctor
./bin/shoutout devices
./bin/shoutout setup --device "Your speaker name"
./bin/shoutout install
```

Alternatively, run a downloaded Linux binary directly with those commands; end users do not need Go. The CI workflow produces amd64 and arm64 binary artifacts. There is no published release or distro package yet.

Installation copies the binary to `~/.local/bin/shoutout`, adds an application-menu entry, and enables a systemd **user** service. It does not require root or alter the desktop's selected output. Start-at-login follows the enabled user service.

## Use

Open **Shoutout** from the application menu or visit [local settings](http://127.0.0.1:17832). Choose a destination and save, then select Shoutout in KDE's sound menu. KDE's application audio controls can route individual applications instead of the whole desktop.

- Receiver volume is limited to **5% absolutely**, enforced in configuration, the UI, and Cast commands.
- Defaults are **1% receiver volume**, **−40 dB attenuation**, and **muted**. Every service restart requires explicit unmuting in settings.
- Desktop volume controls local sink gain. Audio attenuation is applied separately, with a peak bound before encoding.
- Changes to destination, encoding, or settings reconnect playback. Unmuting can therefore take time while the receiver buffers.
- Presets select MP3 bitrate and host queue limits. Receiver-side buffering can add substantial delay beyond those limits.
- A receiver volume increase from another controller is detected by polling; Shoutout gates its audio, mutes, and reconnects muted. This is not a hardware limiter for audio produced by other apps or controls.

The media server binds to the interface used to reach the receiver, on TCP **17833**, and serves only that receiver. Allow that inbound port through the local firewall if necessary. Device discovery uses IPv4 mDNS on UDP **5353**, with manual host/port settings available. Settings bind only to `127.0.0.1:17832`.

The configured output remains available while disconnected. The service retries connectivity errors, but stops trying to claim a receiver after another controller takes over; save settings to reconnect deliberately. One destination is supported at a time. Groups and IPv6-only networks have not been validated.

## Manage

```sh
shoutout status
shoutout configure
systemctl --user status shoutout
journalctl --user -u shoutout
systemctl --user disable --now shoutout  # stop and disable start-at-login
systemctl --user enable --now shoutout  # enable again
shoutout uninstall
```

Uninstall removes the installed binary, desktop entry, and service, while retaining personal settings. Configuration lives at `$XDG_CONFIG_HOME/shoutout/config.json` (normally `~/.config/shoutout/config.json`). Use the running settings page for changes; `setup` is intended before starting the service.

## Develop

```sh
make test
make build
./bin/shoutout run
```

A process lock prevents multiple instances from competing over the virtual output. Stop the installed service before running a development instance. Unit and race tests do not emit audio. Hardware playback testing must keep receiver volume at or below 5% and begin at 1% with attenuation.

See [validation notes](docs/VALIDATION.md) for the tested environment and limitations, and the [implementation plan](docs/PLAN.md) for the design and remaining work.
