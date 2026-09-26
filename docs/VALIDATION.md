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

Hardware testing started muted at 1%; subsequent listening uses 3%, always below the absolute 5% test maximum. This restriction is not a product volume cap.

## Outstanding

AAC acoustic latency, long-duration drift, network recovery, suspend/resume, speaker groups, fresh distro installation and packaged distribution need further validation. One selected receiver/group is supported; multiple simultaneous destinations are not implemented. No automatic game/video synchronization is provided.

The per-user plugin becomes available to the ordinary System Settings launcher after a Plasma login. `shoutout configure` supplies its search path immediately. No privileged system plugin install is required by this method.

GitHub Actions previously refused to start jobs due to an account billing/spending-limit restriction. No hosted workflow steps ran; local checks are independent of that restriction.
