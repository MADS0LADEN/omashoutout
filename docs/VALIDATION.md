# Validation

Local platform: Arch Linux, KDE Plasma 6, PipeWire 1.6.9, Chromecast Audio named Lars Kontor.

## Confirmed

- User confirms Shoutout appears as an output and produces audible audio. The continuous MP3 path had approximately 30 seconds of user-observed delay.
- AAC live segments are accepted by the receiver, with repeated playlist/segment requests and advancing PLAYING status. The user estimates roughly two seconds of audible delay with 500 ms segments, 128 kbps AAC and an 80 ms host buffer. This is a listening estimate, not an instrumented measurement.
- Native capture has `node.virtual=true` and no Pulse client association, allowing KDE to exclude it from the Applications list.
- An isolated sink test confirms native volume affects monitor PCM and native mute produces silence. Service restarts adopt the existing sink without resetting gain or routing.
- The native KCM loads, discovers receivers, displays status and exposes the full 0–100% receiver scale. KDE System Settings enumerates it when the per-user Qt plugin path is loaded.
- Settings use private Unix IPC; the former browser UI has been removed.
- Local Go tests, race checks and vet have passed; native compilation and offscreen UI loading have passed. Synthetic live-stream tests exercise actual encoding, manifest availability and receiver address restrictions without contacting speakers.
- Earlier local amd64/arm64 Go builds and vulnerability checks passed. Native modules require builds for the target Qt/KDE environment.

Agent-run hardware tests started muted at 1%, then used 3%, below the absolute 5% test maximum. This restriction is not a product volume cap.

## Cast Streaming / Opus

- Audio-only receiver application is available on Lars Kontor. Negotiation succeeds with explicit audio format fields and a signed-range SSRC for firmware compatibility.
- Encrypted stereo 48 kHz Opus frames receive advancing acknowledgements. The receiver reported the initial 400 ms buffer, then the user selected 60 ms and receiver feedback reported 60 ms. Neither figure is an acoustic measurement.
- The user confirms audible playback and a noticeable reduction in delay compared with HTTP audio.
- Agent-run hardware setup started muted at 1%, then used 3% receiver volume. The user subsequently adjusted settings; later validation was read-only and preserved those choices. The service remains controlled by native KDE gain/mute.
- Tests exercise AES-CTR against a known vector, packet fragmentation, frame-ID wraparound, RTCP bounds checking, stale feedback rejection, packet retransmission and sender clock mapping. A loopback synthetic test runs the actual encoder, decrypts received packets and sends simulated acknowledgements without contacting speakers.
- Native settings include transport selection and a 40–1000 ms target-delay control. Existing AAC presets remain available; selecting a different transport is explicit.
- Volume-only updates preserve the active session; trim updates reach running PCM capture. Protocol tests verify unmuted volume changes send only a level update and status query, without mute/stop/launch commands.
- Native sliders cover receiver scale, attenuation, host buffer, segment duration and target delay, with synchronized numeric inputs.
- Go race tests, vet and native UI smoke checks pass. Two five-second fuzz runs completed roughly 940,000 RTCP parser cases and 497,000 Ogg parser cases without failures.

## Outstanding

Instrumented acoustic latency, long-duration drift, network recovery, suspend/resume, speaker groups, fresh distro installation and packaged distribution need further validation. One selected receiver/group is supported; multiple simultaneous destinations are not implemented. No automatic game/video synchronization is provided.

The per-user plugin becomes available to the ordinary System Settings launcher after a Plasma login. `shoutout configure` supplies its search path immediately. No privileged system plugin install is required by this method.

GitHub Actions previously refused to start jobs due to an account billing/spending-limit restriction. No hosted workflow steps ran; local checks are independent of that restriction.
