#!/bin/sh

set -eu
umask 022

repository="dire-kiwi/dire-agent"
install_dir=${DIRE_AGENT_INSTALL_DIR-}
requested_version=${DIRE_AGENT_VERSION-latest}
temporary_dir=
staged_file=

usage() {
    cat <<'EOF'
Install Dire Agent from a GitHub release.

Usage:
  install.sh [--install-dir DIR] [--version VERSION]

Options:
  --install-dir DIR  Install to DIR instead of selecting a writable PATH entry.
  --version VERSION  Install a release tag (for example v1.2.3 or 1.2.3).
                     The default is the latest release.
  -h, --help         Show this help.

Environment:
  DIRE_AGENT_INSTALL_DIR  Default value for --install-dir.
  DIRE_AGENT_VERSION      Default value for --version.

Examples:
  curl --proto '=https' --tlsv1.2 -fsSL https://github.com/dire-kiwi/dire-agent/releases/latest/download/install.sh | sh
  curl --proto '=https' --tlsv1.2 -fsSL https://github.com/dire-kiwi/dire-agent/releases/latest/download/install.sh | \
    sh -s -- --install-dir "$HOME/.local/bin" --version v1.2.3
EOF
}

fail() {
    printf 'dire-agent installer: %s\n' "$*" >&2
    exit 1
}

cleanup() {
    if [ -n "$staged_file" ]; then
        rm -f "$staged_file"
    fi
    if [ -n "$temporary_dir" ]; then
        rm -rf "$temporary_dir"
    fi
}

trap cleanup 0
trap 'exit 1' HUP INT TERM

while [ "$#" -gt 0 ]; do
    case "$1" in
        --install-dir)
            [ "$#" -ge 2 ] || fail "--install-dir requires a directory"
            install_dir=$2
            shift 2
            ;;
        --install-dir=*)
            install_dir=${1#*=}
            shift
            ;;
        --version)
            [ "$#" -ge 2 ] || fail "--version requires a release version"
            requested_version=$2
            shift 2
            ;;
        --version=*)
            requested_version=${1#*=}
            shift
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        --)
            shift
            [ "$#" -eq 0 ] || fail "unexpected argument: $1"
            ;;
        *)
            fail "unknown option: $1"
            ;;
    esac
done

[ -n "$requested_version" ] || fail "the requested version cannot be empty"

case "$requested_version" in
    latest) ;;
    v*) ;;
    *) requested_version="v$requested_version" ;;
esac

case "$requested_version" in
    *[!A-Za-z0-9._+-]*) fail "invalid release version: $requested_version" ;;
esac

case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
esac

path_contains() {
    case ":${PATH-}:" in
        *:"$1":*) return 0 ;;
        *) return 1 ;;
    esac
}

default_install_dir() {
    if [ -n "${HOME-}" ]; then
        for candidate in "$HOME/.local/bin" "$HOME/bin"; do
            if path_contains "$candidate" && [ -d "$candidate" ] &&
                [ -w "$candidate" ] && [ -x "$candidate" ]; then
                printf '%s\n' "$candidate"
                return 0
            fi
        done
    fi

    if path_contains /usr/local/bin && [ -d /usr/local/bin ] &&
        [ -w /usr/local/bin ] && [ -x /usr/local/bin ]; then
        printf '%s\n' /usr/local/bin
        return 0
    fi

    old_ifs=$IFS
    IFS=:
    set -f
    for candidate in ${PATH-}; do
        case "$candidate" in
            /*)
                if [ -d "$candidate" ] && [ -w "$candidate" ] && [ -x "$candidate" ]; then
                    printf '%s\n' "$candidate"
                    return 0
                fi
                ;;
        esac
    done
    set +f
    IFS=$old_ifs

    if [ -n "${HOME-}" ]; then
        printf '%s\n' "$HOME/.local/bin"
        return 0
    fi

    return 1
}

if [ -z "$install_dir" ]; then
    install_dir=$(default_install_dir) ||
        fail "no writable directory was found on PATH; pass --install-dir DIR"
fi

[ -n "$install_dir" ] || fail "the install directory cannot be empty"
case "$install_dir" in
    /*) ;;
    *) install_dir="./$install_dir" ;;
esac

mkdir -p "$install_dir" || fail "could not create install directory: $install_dir"
install_dir=$(
    cd "$install_dir" || exit 1
    pwd -P
) || fail "could not resolve install directory: $install_dir"

[ -w "$install_dir" ] && [ -x "$install_dir" ] ||
    fail "install directory is not writable: $install_dir"

temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/dire-agent.XXXXXX") ||
    fail "could not create a temporary directory"

download() {
    url=$1
    destination=$2

    if command -v curl >/dev/null 2>&1; then
        curl --proto '=https' --tlsv1.2 -fsSL --retry 3 --retry-delay 1 -o "$destination" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -q -O "$destination" "$url"
    else
        fail "curl or wget is required to download Dire Agent"
    fi
}

release_root="https://github.com/$repository/releases"
if [ "$requested_version" = latest ]; then
    download "$release_root/latest/download/version.txt" "$temporary_dir/version.txt"
    version=$(sed -n '1{s/[[:space:]]*$//;p;}' "$temporary_dir/version.txt")
else
    version=$requested_version
fi

case "$version" in
    v[A-Za-z0-9._+-]*) ;;
    *) fail "the release returned an invalid version: $version" ;;
esac
case "$version" in
    *[!A-Za-z0-9._+-]*) fail "the release returned an invalid version: $version" ;;
esac

asset="dire-agent-$os-$arch"
release_url="$release_root/download/$version"
download "$release_url/$asset" "$temporary_dir/$asset"
download "$release_url/checksums.txt" "$temporary_dir/checksums.txt"

expected_checksum=$(awk -v asset="$asset" '
    {
        filename = $2
        sub(/^\*/, "", filename)
        if (filename == asset) {
            print $1
            exit
        }
    }
' "$temporary_dir/checksums.txt")
[ -n "$expected_checksum" ] || fail "checksums.txt does not contain $asset"

if command -v sha256sum >/dev/null 2>&1; then
    actual_checksum=$(sha256sum "$temporary_dir/$asset" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    actual_checksum=$(shasum -a 256 "$temporary_dir/$asset" | awk '{print $1}')
else
    fail "sha256sum or shasum is required to verify the download"
fi

[ "$actual_checksum" = "$expected_checksum" ] ||
    fail "checksum verification failed for $asset"

target="$install_dir/dire-agent"
[ ! -d "$target" ] || fail "install target is a directory: $target"

staged_file=$(mktemp "$install_dir/.dire-agent.XXXXXX") ||
    fail "could not create a staging file in $install_dir"
cp "$temporary_dir/$asset" "$staged_file"
chmod 755 "$staged_file"
[ ! -d "$target" ] || fail "install target became a directory: $target"
mv -f "$staged_file" "$target"
staged_file=

printf 'Installed dire-agent %s to %s/dire-agent\n' "$version" "$install_dir"
if ! path_contains "$install_dir"; then
    printf 'Add %s to PATH to run dire-agent (no shell files were modified).\n' "$install_dir"
fi
