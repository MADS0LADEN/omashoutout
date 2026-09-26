# Project instructions

- Keep all checkouts, names, links, comparisons, and notes about other solutions inside the ignored `research/` directory. Do not mention those solutions in tracked code, comments, documentation, commit messages, or GitHub descriptions.
- Do not copy or vendor reference implementations into the project. Evaluate dependency and attribution requirements before adopting code.
- Deliver working playback, KDE output selection, configurable destinations, volume scaling, presets, and easy installation first. Optimize measured end-to-end latency after the functional baseline; latency targets must not block feature implementation.
- Go is the proposed language. Reconsider Rust only when measurements or integration requirements establish a concrete advantage.
- Distinguish planned behavior from implemented and hardware-tested behavior.
- Keep research local and untracked; never force-add it.

- Hardware testing: receiver volume MUST NEVER exceed 5% (0.05) during agent-run speaker tests. Start muted at 1%, verify reported volume before non-silent testing, and retain appropriate test-signal attenuation. This is a testing restriction, NOT a product volume limit; the user can configure the full receiver range.
- Use native KDE device volume/mute and native KDE settings. Do not provide a web settings interface. Internal audio transport must not appear as a normal application in KDE's mixer.
