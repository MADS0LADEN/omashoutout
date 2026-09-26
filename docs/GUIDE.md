# Installation and usage

## Requirements

The **daemon** is the same on Omarchy and KDE: a PipeWire null sink named `shoutout` (description ShoutOut). Desktop volume and mute apply to that sink. Capture stays a virtual node and must not appear as an application in the mixer.

### Omarchy

Runtime: PipeWire's PulseAudio compatibility service, `pactl`, `parec`, FFmpeg with AAC, MP3 and `libopus` encoders, a systemd user session, and the Omarchy shell (`omarchy-shell` for the settings panel). Build: Go (see `go.mod`) only — `make build` without the KDE target.

### KDE Plasma 6

Runtime: the same PipeWire, `pactl`, `parec`, and FFmpeg requirements, plus a KDE Plasma 6 desktop and systemd user session. Build: Go, CMake, Ninja, a C++20 compiler, Qt6 Widgets and Network, and KDE Frameworks 6 KCMUtils and CoreAddons development files.

Arch is the current development platform; other distributions need validation.

## Install

For Arch Linux, the [PKGBUILD](ARCH.md) installs system-wide, including the native KDE module only. It does not install the Omarchy shell plugin. The instructions below describe per-user installation.

### Omarchy

```sh
make build
./build/shoutout doctor
./build/shoutout setup --device "Your speaker name"
./build/shoutout install
shoutout configure
```

`shoutout install` copies the binary to `~/.local/bin/shoutout`, installs the user systemd service, and copies the Omarchy plugin to `~/.config/omarchy/plugins/io.github.lkarlslund.shoutout/`, enabling it on the right side of the shell bar. It does not require the KDE module.

### KDE (per-user)

```sh
make build kde
./build/shoutout doctor
./build/shoutout setup --device "Your speaker name"
./build/shoutout install
shoutout configure
```

The per-user KDE installer also copies the native settings module, desktop launcher and Plasma environment script, then enables the systemd user service. It preserves the selected output and existing ShoutOut volume/mute. A newly created output starts muted at a default receiver scale of 1%.

### Configure

`shoutout configure` opens the Omarchy settings panel via `omarchy-shell io.github.lkarlslund.shoutout open` when `omarchy-shell` is on PATH. Otherwise it opens the ShoutOut module inside KDE System Settings. The Omarchy panel is the intended Omarchy settings surface; playback still depends on the same daemon. Hardware validation recorded in the docs was on KDE Plasma — the Omarchy panel has not been hardware-tested.

On KDE, the regular System Settings launcher discovers the per-user plugin after the next Plasma login. Close an already open System Settings window before using the new launcher. A system package can install the plugin into the standard Qt6 plugin directory; the Go-only CI artifacts do not include the native KDE module.

## Use

Choose a detected receiver or enter `address:port` (for example `192.168.1.10:8009`). The service discovers receivers continuously from startup and pushes live changes to the settings dropdown. Devices disappear after 45 seconds without refreshed discovery records, or earlier when they announce departure. Your selected destination is retained if it becomes unavailable. One destination is supported at a time; advertised speaker groups can be selected but have not been validated. Select the **ShoutOut** output and unmute it in the Omarchy Audio panel or in KDE's audio controls, then route your applications normally.

- The desktop output slider and mute for ShoutOut are authoritative. Internal capture is marked virtual so it does not appear as an application in the mixer.
- Receiver volume scale is configurable from **0–100%**. Keep it low for sensitive speakers. The 5% maximum applies only to development speaker tests.
- Native volume changes apply to captured audio, so their audible effect includes stream delay. Native mute also sends a receiver mute command without restarting playback.
- Encoding choices are **Cast Streaming / Opus (experimental)** and AAC live segments. Existing MP3 configurations remain supported as a legacy option. Cast Streaming sends encrypted, paced Opus frames (5 ms below a 40 ms target, 10 ms below 80 ms, otherwise 20 ms) over UDP, with receiver feedback and bounded retransmission. Its target-delay control accepts 10–1000 ms; the Balanced target is 100 ms. This is a requested receiver buffer, not measured end-to-end latency. Select AAC manually if the receiver does not support this mode.
- For AAC, live segment duration is configurable from 250–2000 ms. The live playlist holds six segments; receiver buffering is additional. MP3 can have very high receiver delay.
- **Low latency** selects Opus at 128 kbps with a 20 ms receiver target. **Balanced** (the new-install default) selects Opus at 192 kbps with a 100 ms target. **High quality** selects AAC at 320 kbps with 500 ms segments. These durations are protocol settings, not measured audible latency. Editing encoding, bitrate or buffering selects Custom. Existing configurations retain their transport settings.
- Volume scale applies to the running session without reconnecting. Volume changes during unmuted playback avoid a mute cycle. Numeric controls accompany sliders for scale, segment length and target delay.
- Capture batching and transport queues are managed internally. There is no separate host-buffer or attenuation setting.
- Applying destination, encoding, segment-length or target-delay changes restarts the stream. The sink and desktop routing remain in place. Another controller taking over the receiver stops automatic reconnection; apply settings to reclaim it deliberately.

Settings use a private Unix socket in `$XDG_RUNTIME_DIR`, with no browser interface. The Omarchy shell widget and the KDE module are native UIs, not web pages. Cast Streaming uses a negotiated UDP endpoint on the receiver, with return feedback to the sender’s ephemeral port. For AAC/MP3, the media server listens on the receiver-facing interface at TCP **17833**, with a random session URL and receiver-address restriction. Discovery uses IPv4 mDNS on UDP **5353**. Allow those network paths when needed; the installer does not change your firewall.

## Manage and develop

```sh
shoutout configure
shoutout status
shoutout devices
journalctl --user -u shoutout
systemctl --user restart shoutout
shoutout uninstall
make test
```

Configuration is stored in `$XDG_CONFIG_HOME/shoutout/config.json`, normally `~/.config/shoutout/config.json`. `setup` is intended before starting the service; use native settings or `shoutout config` / `shoutout apply` while running.

`shoutout uninstall` stops the user service, removes the sink, the binary, the desktop entry, and the Omarchy plugin when present. Personal settings in `~/.config/shoutout/config.json` stay. On KDE, restart Plasma after uninstall to clear an inherited plugin search path from a per-user module install.

Tests use synthetic PCM and protocol simulations without emitting audio. Agent-run speaker tests must begin muted at 1%, verify receiver status, and never exceed 5%. See [validation](VALIDATION.md) and the [plan](PLAN.md).
