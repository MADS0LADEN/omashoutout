# Implementation plan

## Implemented architecture

The Go service owns discovery, Cast session control, persistence and supervision. Settings are exposed through the Omarchy shell bar plugin or the Qt6/KDE Frameworks System Settings module; both talk to the service over the same private Unix socket. The audio device itself uses the desktop's PipeWire session (native gain and mute on the Omashoutout sink).

```mermaid
flowchart LR
    Applications --> Sink[Omashoutout output: native gain and mute]
    Sink --> Capture[Virtual monitor capture]
    Capture --> PCM[Float PCM mute, fade-in and peak bound]
    PCM --> Encoder[FFmpeg Opus, AAC or MP3]
    Encoder --> Transport[Encrypted UDP or receiver-restricted HTTP]
    Transport --> Receiver[Google audio device]
    Settings[Omarchy plugin or KDE module] --> IPC[Private Unix socket]
    IPC --> Service[Go service]
    Service --> Receiver
```

A stable owned null sink survives service restarts to preserve routing and native gain/mute. Capture is pinned to its monitor and marked virtual. Native mute is mirrored to the receiver; volume is applied once by the audio server, with independent user-selected receiver scaling. New connections verify muted receiver volume before enabling non-silent PCM.

Cast Streaming negotiates the built-in audio-only receiver, stereo 48 kHz Opus, a UDP endpoint and an explicit playback-delay target. Frames are encrypted independently with session keys, packetized, paced, acknowledged and retained only inside a bounded retransmission window. A stalled acknowledgement stream or a late encoder triggers reconnection rather than an increasing backlog. Requested/reported buffer durations are shown separately from audible latency.

AAC live delivery uses a sliding six-segment playlist, bounded retained files, tokenized URLs and prompt segment publication. Continuous MP3 remains selectable. Capture requests 5 ms batches for Cast Streaming and 40 ms for HTTP delivery. Cast Streaming feeds float PCM through a streaming WAV header with 5 ms demux packets and 5, 10 or 20 ms Opus frames selected from the receiver target; the MP3 queue is bounded internally. Live segment duration remains adjustable.

A background discovery worker maintains expiring device records and publishes snapshots over a persistent private socket subscription. Each settings client updates its destination list without replacing unsaved settings; reconnects use the same discovery cache.

The native settings UIs (Omarchy plugin and KDE module) configure destination discovery/manual `address:port`, full-range receiver volume scale, encoding, bitrate, segment duration, presets and enablement. They show connection status and native mute/volume on the Omashoutout sink. Volume scale applies live; no-op and inactive transport settings do not reconnect. Active transport changes restart the session. Numeric inputs and sliders stay synchronized. Settings travel through a mode-0600 Unix socket. There is no web settings UI.

## Remaining milestones

1. Measure audible delay for Cast Streaming at 400, 200 and 100 ms requested targets, including loss recovery and long-duration drift. Also measure audible delay on Lars Kontor with AAC live segments, including start/stop and drift over 30 minutes. Compare 250, 500 and 1000 ms segments. Distinguish receiver timeline estimates from acoustic measurement. The user observed about 30 seconds with continuous MP3.
2. Validate reconnects, audio-server restart, suspend/resume, takeover and external volume changes on hardware. Retain bounded buffers and prevent stale audio replay.
3. Validate advertised speaker groups. Multiple independent destinations and synchronized playback across them are not implemented.
4. The Arch PKGBUILD installs the KCM in the standard plugin directory with declared runtime dependencies. Validate installation and removal in a clean Arch environment, then extend packaging to other distributions.
5. Tune the implemented real-time transport using receiver feedback and measured acoustic results. Do not label multi-second audio interactive or imply automatic video synchronization.

Go remains appropriate for service and network control. Changing language does not itself remove receiver buffering. Keep the small native KDE integration separate; reconsider core language only if measured implementation constraints justify it.

## Verification

Run Go tests with the race detector and vet, native module build/load checks, and synthetic FFmpeg integration tests. Hardware tests are separate: begin muted at 1%, verify volume, and never exceed 5%. Users retain the full configurable receiver range.

Acoustic latency measurement should compare source and receiver output on a common clock where possible, reporting median, p95, drift and uncertainty. Cast PLAYING status, network RTT and encoding queue lengths cannot establish audible end-to-end latency.
