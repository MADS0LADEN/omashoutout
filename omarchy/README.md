# ShoutOut (Omarchy bar plugin)

Settings panel for the ShoutOut daemon: pick a Cast destination, set receiver volume scale, choose playback presets, and adjust encoding. Desktop volume and mute stay on the ShoutOut PipeWire output in the Omarchy **Audio** panel — this plugin does not duplicate those controls.

## Install

After the `shoutout` binary is built or packaged, run once:

```bash
shoutout install
```

Then enable the plugin in Omarchy shell settings, or open the panel with:

```bash
shoutout configure
```

You can also click the speaker icon on the bar.

## Remove

```bash
shoutout uninstall
```

Disable or remove the plugin from Omarchy shell settings if you added it manually with `omarchy plugin add`.
