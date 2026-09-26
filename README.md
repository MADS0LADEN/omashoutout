# ShoutOut

A Linux virtual audio output that sends desktop audio to a Google Cast audio device. Select **Shoutout** in KDE's normal output selector. Configure the destination, encoding, buffering and volume scale in the native **ShoutOut** System Settings module.

Playback is confirmed on a Chromecast Audio. **Cast Streaming / Opus** now produces audible audio with noticeably less delay than HTTP delivery. The test receiver reports the user's selected **60 ms playback buffer**; actual end-to-end latency remains unmeasured. AAC live segments remain the default compatibility mode (roughly two seconds in an earlier listening test), while continuous MP3 exhibited roughly 30 seconds. No automatic video synchronization is provided.

## Install

Requires a KDE Plasma 6 desktop, a systemd user session, PipeWire's PulseAudio compatibility service, `pactl`, `parec`, and FFmpeg with AAC, MP3 and `libopus` encoders. Building also requires Go (see `go.mod`), CMake, Ninja, a C++20 compiler, Qt6 Widgets, and KDE Frameworks 6 KCMUtils and CoreAddons development files. Arch is the current development platform; other distributions need validation.

```sh
make build kde
./bin/shoutout doctor
./bin/shoutout setup --device "Your speaker name"
./bin/shoutout install
shoutout configure
```

The per-user installer copies the service binary, native settings plugin, desktop launcher and Plasma environment script, then enables the systemd user service. It preserves the selected output and existing Shoutout volume/mute. A newly created output starts muted at a default receiver scale of 1%.

`shoutout configure` opens the ShoutOut module inside KDE System Settings immediately. The regular System Settings launcher discovers the per-user plugin after the next Plasma login. Close an already open System Settings window before using the new launcher. A system package can instead install the plugin into the standard Qt6 plugin directory; no distro package or complete binary release is published yet. The Go-only CI artifacts do not include the native module.

## Use

Choose a detected receiver or enter `address:port` (for example `192.168.1.10:8009`). The service discovers receivers continuously from startup and pushes live changes to the settings dropdown. Devices disappear after 45 seconds without refreshed discovery records, or earlier when they announce departure. Your selected destination is retained if it becomes unavailable. One destination is supported at a time; advertised speaker groups can be selected but have not been validated. Select Shoutout in KDE's audio selector, unmute it and route your applications normally.

- KDE's normal output slider and mute are authoritative. Internal capture is marked virtual so it does not appear as an application in KDE's mixer.
- Receiver volume scale is configurable from **0–100%**. Keep it low for sensitive speakers. The 5% maximum applies only to development speaker tests.
- Native volume changes apply to captured audio, so their audible effect includes stream delay. Native mute also sends a receiver mute command without restarting playback.
- Encoding choices are **Cast Streaming / Opus (experimental)**, AAC live segments and continuous MP3. Cast Streaming sends encrypted, paced 20 ms Opus frames over UDP, with receiver feedback and bounded retransmission. Its target-delay control accepts 40–1000 ms; the initial target is 400 ms. This is a requested receiver buffer, not measured end-to-end latency. Select AAC manually if the receiver does not support this mode.
- For AAC, live segment duration is configurable from 250–2000 ms. The live playlist holds six segments; receiver buffering is additional. MP3 can have very high receiver delay.
- Interactive, Video and Music presets select progressively longer AAC segments and encoding bitrates. These presets use AAC delivery. Select Cast Streaming explicitly to configure its target delay. Preset names describe intent, not measured latency guarantees. Custom settings are supported.
- Volume scale applies to the running session without reconnecting. Volume changes during unmuted playback avoid a mute cycle. Numeric controls accompany sliders for scale, segment length and target delay.
- Capture batching and transport queues are managed internally. There is no separate host-buffer or attenuation setting.
- Applying destination, encoding, segment-length or target-delay changes restarts the stream. The sink and desktop routing remain in place. Another controller taking over the receiver stops automatic reconnection; apply settings to reclaim it deliberately.

Settings use a private Unix socket in `$XDG_RUNTIME_DIR`, with no browser interface. Cast Streaming uses a negotiated UDP endpoint on the receiver, with return feedback to the sender’s ephemeral port. For AAC/MP3, the media server listens on the receiver-facing interface at TCP **17833**, with a random session URL and receiver-address restriction. Discovery uses IPv4 mDNS on UDP **5353**. Allow those network paths when needed; the installer does not change your firewall.

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

Configuration is stored in `$XDG_CONFIG_HOME/shoutout/config.json`, normally `~/.config/shoutout/config.json`. `setup` is intended before starting the service; use native settings or `shoutout config` / `shoutout apply` while running. Uninstall retains personal settings and removes the owned audio sink and installed files. Restart Plasma after uninstall to clear its inherited plugin search path.

Tests use synthetic PCM and protocol simulations without emitting audio. Agent-run speaker tests must begin muted at 1%, verify receiver status, and never exceed 5%. See [validation](docs/VALIDATION.md) and the [plan](docs/PLAN.md).
