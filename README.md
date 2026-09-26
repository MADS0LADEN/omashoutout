# Shoutout

A planned Linux virtual audio output for Google Cast audio devices, with KDE integration, low-latency streaming, adjustable volume scaling for sensitive speakers, and Interactive/Video/Music presets.

**Status: design and feasibility stage. No working application or installer yet.**

The intended workflow is to install Shoutout, choose a destination and volume limit in its settings, then select **Shoutout** in KDE's audio output menu. Applications can also be routed individually using the desktop's audio controls.

Go is the proposed implementation language. The initial target is Linux with PipeWire and its PulseAudio compatibility service. Low end-to-end latency is a primary requirement; actual receiver measurements will determine the streaming transport before the rest of the application is built.

See the [implementation plan](docs/PLAN.md) for components, milestones, volume behavior, installation, and acceptance criteria.
