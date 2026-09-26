# Arch Linux package

Build and install the latest Git version system-wide:

```sh
sudo pacman -S --needed base-devel git
git clone https://github.com/lkarlslund/shoutout.git
cd shoutout/packaging/arch
makepkg -si
systemctl --user enable --now shoutout.service
shoutout configure
```

The package is named `shoutout-git`. It installs:

- `/usr/bin/shoutout`
- The native KDE module under `/usr/lib/qt6/plugins/`
- `/usr/lib/systemd/user/shoutout.service`
- A desktop launcher, documentation and license notices under `/usr/share/`

All users can access the installed program and settings page. Each user enables
their own service and chooses their destination. Audio runs in that user's
PipeWire session, not as a root daemon. The package does not start playback or
modify anyone's home directory during installation. The system-installed KDE
module needs no per-user plugin-path setup; reopen System Settings after install.

Choose a destination and unmute ShoutOut in KDE's output selector. Do not run
`shoutout install` for a package installation; that command installs a separate
copy in your home directory.

## Replacing a per-user installation

Close System Settings, then remove the older per-user copy before enabling the
packaged service:

```sh
~/.local/bin/shoutout uninstall
systemctl --user daemon-reload
systemctl --user enable --now shoutout.service
/usr/bin/shoutout configure
```

Personal settings are retained. Log out and back in to clear the old inherited
Qt plugin path. The package never deletes per-user files automatically.

## Build only or remove

`makepkg -s` builds the `.pkg.tar.zst` without installing it. `check()` runs Go
race tests and vet; these use simulated receivers and do not play speaker audio.
This recipe tracks Git `main`; it has not been submitted to the AUR.

To remove it, first run `systemctl --user disable --now shoutout.service`, then
`sudo pacman -Rns shoutout-git`. Personal settings remain. An existing virtual
output disappears when the audio session ends.
