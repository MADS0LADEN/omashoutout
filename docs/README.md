# Omashoutout

**Use a Cast speaker as the system audio output on Omarchy**

Omashoutout adds an **Omashoutout** PipeWire output. Select it in the Omarchy **Audio** panel and play games, the browser, a music player, or the whole desktop through a Cast receiver.

Settings are the Omashoutout bar plugin (speaker icon, or `omashoutout configure`). Volume and mute stay on that output in the Audio panel. The plugin does not duplicate those controls.

- **Bar plugin.** Destination, receiver volume scale, playback preset, and encoding.
- **Speakers appear automatically.** Live discovery keeps the destination list up to date. You can also type `host:port`.
- **Presets.** Low latency, Balanced, High quality, or Custom.
- **Receiver volume scale.** 0–100% of the receiver at full desktop volume, applied without reconnecting.

| Preset | Playback |
|---|---|
| Low latency | Opus · 128 kbps · 20 ms receiver target |
| Balanced | Opus · 192 kbps · 100 ms receiver target |
| High quality | AAC · 320 kbps · 500 ms live segments |

Receiver targets and segment lengths are protocol settings, not measured end-to-end latency. Cast Streaming (Opus) is experimental. AAC live segments are the other encoding. An existing MP3 configuration stays available as a legacy option.

## Get started

Omashoutout needs **PipeWire** with PulseAudio compatibility, `pactl`, `parec`, FFmpeg, a systemd user session, and the Omarchy shell. Details are in [installation and usage](GUIDE.md).

```sh
make build
./build/omashoutout install
omashoutout configure
```

`omashoutout install` copies the binary to `~/.local/bin/omashoutout`, installs the user systemd service, and enables the bar plugin `io.github.MADS0LADEN.omashoutout` on the right side of the shell bar.

In the panel, enable the device, choose a speaker (or enter an address such as `192.168.1.10:8009`), set the receiver volume scale, and apply. Then select **Omashoutout** in the Audio panel and unmute it. Start with a low volume scale on a sensitive speaker.

A new output starts muted, with a default receiver scale of 1%. Balanced is the preset for a new install.

The same daemon can open a KDE Plasma 6 System Settings page. That path is documented in [Arch packaging](ARCH.md) and [installation and usage](GUIDE.md). Hardware notes in the docs were recorded on KDE; the Omarchy panel has not been hardware-tested.

[Usage & troubleshooting](GUIDE.md) · [Development plan](PLAN.md) · [Validation](VALIDATION.md)

[MIT licensed](../LICENSE). [Third-party notices](DEPENDENCIES.md).
