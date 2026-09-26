# ShoutOut

**Use your Google Audio compatible speakers for system audio**

ShoutOut turns a Google Cast speaker into a Linux audio output. Select it in KDE’s audio menu and send sound from your games, browser, music player—or your whole desktop.

![ShoutOut settings in KDE](shoutout.png)

- **Feels native.** A dedicated System Settings page, with KDE’s familiar volume and mute controls.
- **Speakers appear automatically.** Live discovery keeps your destination list up to date.
- **Choose your balance.** Low latency, Balanced and High quality presets, plus custom controls.
- **Tame sensitive speakers.** Adjustable volume scaling, applied without interrupting playback.

| Preset | Playback |
|---|---|
| Low latency | Opus · 128 kbps · 40 ms receiver target |
| Balanced | Opus · 192 kbps · 100 ms receiver target |
| High quality | AAC · 320 kbps |

Receiver targets are buffer settings, not total audible latency. Playback is tested on Chromecast Audio; Cast Streaming is experimental.

## Get started

For **KDE Plasma 6 + PipeWire**. Build from source with Go, FFmpeg and the Qt6/KDE development libraries; see [requirements and installation details](GUIDE.md).

On Arch Linux, use the [system-wide package](ARCH.md):

```sh
make package-install
shoutout configure
```

Or install from source for your user:

```sh
make build kde
./build/shoutout install
shoutout configure
```

Choose your speaker, select **ShoutOut** as your KDE audio output, and unmute. Start with a low volume scale for sensitive speakers.

The settings page opens immediately through `shoutout configure`. Per-user source installs need a new login for discovery through the normal System Settings launcher. Arch packages are discoverable immediately.

[Usage & troubleshooting](GUIDE.md) · [Development plan](PLAN.md) · [Validation](VALIDATION.md)

[MIT licensed](../LICENSE). [Third-party notices](DEPENDENCIES.md).
