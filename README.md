# Shoutout

A planned Linux virtual audio output for Google Cast audio devices, with KDE integration, low-latency streaming, adjustable volume scaling for sensitive speakers, and Interactive/Video/Music presets.

**Status: implementation planned. No working application or installer yet.**

The intended workflow is to install Shoutout, choose a destination and volume limit in its settings, then select **Shoutout** in KDE's audio output menu. Applications can also be routed individually using the desktop's audio controls.

Go is the proposed implementation language. The initial target is Linux with PipeWire and its PulseAudio compatibility service. Build working playback, device selection, volume scaling, presets, and installation first. Then measure and optimize end-to-end latency on the working system; initial playback may have noticeable buffering.

See the [implementation plan](docs/PLAN.md) for components, milestones, volume behavior, installation, and acceptance criteria.
