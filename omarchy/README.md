# Omashoutout (Omarchy bar plugin)

Settings panel for the Omashoutout daemon: pick a Cast destination, set receiver volume scale, choose playback presets, and adjust encoding. Desktop volume and mute stay on the Omashoutout PipeWire output in the Omarchy **Audio** panel — this plugin does not duplicate those controls.

## Install

After the `omashoutout` binary is built or packaged, run once:

```bash
omashoutout install
```

Then enable the plugin in Omarchy shell settings, or open the panel with:

```bash
omashoutout configure
```

You can also click the speaker icon on the bar.

## Remove

```bash
omashoutout uninstall
```

Disable or remove the plugin from Omarchy shell settings if you added it manually with `omarchy plugin add`.
