# Omashoutout (Omarchy bar plugin)

Settings for the Omashoutout daemon on the Omarchy shell bar.

The panel sets:

- whether the device is enabled
- the Cast destination (discovered speakers, or a `host:port` address)
- receiver volume at full desktop volume (0–100%), applied without reconnecting
- a playback preset: Low latency, Balanced, High quality, or Custom
- encoding: Cast Streaming / Opus (experimental) or AAC live segments
- AAC live segment length (250–2000 ms) and the Opus receiver target delay (10–1000 ms)
- encoding bitrate (128–320 kbps)

Desktop volume and mute stay on the **Omashoutout** output in the Omarchy **Audio** panel.

## Install

After the `omashoutout` binary is built or packaged, run once:

```bash
omashoutout install
```

That copies this plugin to `~/.config/omarchy/plugins/io.github.MADS0LADEN.omashoutout/` and enables it on the right side of the bar. Open it from the speaker icon, from Omarchy shell settings, or with:

```bash
omashoutout configure
```

## Remove

```bash
omashoutout uninstall
```

Disable or remove the plugin from Omarchy shell settings if you added it manually with `omarchy plugin add`.
