# Arch Linux package

Build and install the latest Git version system-wide:

```sh
sudo pacman -S --needed base-devel git
git clone https://github.com/lkarlslund/shoutout.git
cd shoutout
make package-install
shoutout configure
```

The root [PKGBUILD](../PKGBUILD) builds `shoutout-git`. It installs:

- `/usr/bin/shoutout`
- The native KDE module under `/usr/lib/qt6/plugins/`
- `/usr/lib/systemd/user/shoutout.service`
- A desktop launcher, documentation and license notices under `/usr/share/`

Installation enables ShoutOut for user sessions and starts it immediately for
logged-in users. Future sessions start it automatically. Audio runs as each
user, with their own settings and PipeWire session. Open ShoutOut in System
Settings and choose your speaker; no service setup or reboot is needed.

The native module is installed in KDE's standard plugin directory. Reopen System
Settings if it was already open during installation.

Choose a destination and unmute ShoutOut in KDE's output selector. Do not run
`shoutout install` for a package installation; that command installs a separate
copy in your home directory.

## Build only or remove

`make package` builds the `.pkg.tar.zst` in `build/` without installing it.
All package working files also stay under `build/`. The root `PKGBUILD`
can still be used directly with `makepkg`. `check()` runs Go
race tests and vet; these use simulated receivers and do not play speaker audio.
This recipe tracks Git `main`; it has not been submitted to the AUR.

Remove it with `sudo pacman -Rns shoutout-git`; the package stops its running
user services and removes automatic startup. Personal settings remain. An existing virtual
output disappears when the audio session ends.
