# Maintainer: Lars Karlslund
pkgname=omashoutout-git
pkgver=0.1.0.r33.g7a4bcfa
pkgrel=1
pkgdesc='Use your Google Audio compatible speakers for system audio, with native KDE settings'
arch=('x86_64')
url='https://github.com/MADS0LADEN/omashoutout'
license=('MIT')
install=omashoutout.install
depends=('ffmpeg' 'libpulse' 'pipewire-pulse' 'qt6-base' 'kcmutils'
         'kcoreaddons' 'systemsettings' 'plasma-pa' 'systemd' 'glibc' 'libgcc' 'libstdc++')
makedepends=('git' 'go>=2:1.26' 'cmake' 'ninja')
provides=('omashoutout')
conflicts=('omashoutout')
source=('omashoutout::git+https://github.com/MADS0LADEN/omashoutout.git#branch=main')
sha256sums=('SKIP')

pkgver() {
  cd "$srcdir/omashoutout"
  printf '0.1.0.r%s.g%s' "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)"
}

prepare() {
  cd "$srcdir/omashoutout"
  GOTOOLCHAIN=local go mod download
}

build() {
  cd "$srcdir/omashoutout"
  CGO_ENABLED=1 GOTOOLCHAIN=local go build -trimpath -buildmode=pie -mod=readonly \
    -ldflags "-linkmode=external -extldflags '${LDFLAGS}' -X main.version=${pkgver}" \
    -o build/omashoutout ./cmd/omashoutout
  cmake -S kde -B build/kde -G Ninja \
    -DCMAKE_BUILD_TYPE=None -DCMAKE_INSTALL_PREFIX=/usr
  cmake --build build/kde
}

check() {
  cd "$srcdir/omashoutout"
  GOTOOLCHAIN=local go test -race ./...
  GOTOOLCHAIN=local go vet ./...
}

package() {
  cd "$srcdir/omashoutout"
  install -Dm755 build/omashoutout "$pkgdir/usr/bin/omashoutout"
  DESTDIR="$pkgdir" cmake --install build/kde
  install -Dm644 packaging/omashoutout.service "$pkgdir/usr/lib/systemd/user/omashoutout.service"
  install -d "$pkgdir/usr/lib/systemd/user/default.target.wants"
  ln -s ../omashoutout.service "$pkgdir/usr/lib/systemd/user/default.target.wants/omashoutout.service"
  install -Dm644 packaging/omashoutout.desktop "$pkgdir/usr/share/applications/omashoutout.desktop"
  install -Dm644 LICENSE "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
  install -Dm644 licenses/*.txt -t "$pkgdir/usr/share/licenses/$pkgname/"
  install -Dm644 docs/*.md docs/*.png -t "$pkgdir/usr/share/doc/omashoutout/"
}
