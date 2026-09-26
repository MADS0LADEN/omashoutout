# Arch Linux package

The packaged **KDE System Settings module** is separate from the **Omarchy shell bar plugin** (`io.github.MADS0LADEN.omashoutout`). The plugin is installed per user by `./build/omashoutout install` on Omarchy and is not part of this package recipe.

Build and install the latest Git version system-wide:

```sh
sudo pacman -S --needed base-devel git
git clone https://github.com/MADS0LADEN/omashoutout.git
cd omashoutout
make package-install
omashoutout configure
```

The root [PKGBUILD](../PKGBUILD) builds `omashoutout-git`. It installs:

- `/usr/bin/omashoutout`
- The native KDE module under `/usr/lib/qt6/plugins/`
- `/usr/lib/systemd/user/omashoutout.service`
- A desktop launcher, documentation and license notices under `/usr/share/`

Installation enables Omashoutout for user sessions and starts it immediately for
logged-in users. Future sessions start it automatically. Audio runs as each
user, with their own settings and PipeWire session. Open Omashoutout in System
Settings and choose your speaker; no service setup or reboot is needed.

The native module is installed in KDE's standard plugin directory. Reopen System
Settings if it was already open during installation.

Choose a destination and unmute Omashoutout in KDE's output selector. Do not run
`omashoutout install` for a package installation; that command installs a separate
copy in your home directory.

## Build only or remove

`make package` builds the `.pkg.tar.zst` in `build/` without installing it.
All package working files also stay under `build/`. The root `PKGBUILD`
can still be used directly with `makepkg`. `check()` runs Go
race tests and vet; these use simulated receivers and do not play speaker audio.
This recipe tracks Git `main`; it has not been submitted to the AUR.

Remove it with `sudo pacman -Rns omashoutout-git`; the package stops its running
user services and removes automatic startup. Personal settings remain. An existing virtual
output disappears when the audio session ends.


## Dependencies

Runtime dependencies cover FFmpeg encoding, `pactl`/`parec` from `libpulse`,
PipeWire's PulseAudio server, Qt6 and KDE libraries, System Settings, KDE's
`plasma-pa` audio controls, systemd, and the directly linked C/C++ runtimes.
PipeWire pulls in its session-manager dependency. Discovery runs inside
Omashoutout and does not require a separate discovery service.

Build dependencies are Git, Go 1.26 or newer, CMake and Ninja, on top of Arch's
standard `base-devel` tools. Arch's Go package uses epoch `2`, so the version
constraint is `go>=2:1.26`. FFmpeg supplies AAC and Opus encoding; separate
encoder development packages are not needed.
