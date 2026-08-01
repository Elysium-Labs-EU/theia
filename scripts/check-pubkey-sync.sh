#!/bin/sh
# Fails if the release signing public key embedded in cmd/update.go
# (releaseSigningPublicKeyPEM) and install.sh (RELEASE_SIGNING_PUBKEY) have
# drifted apart. Both copies must verify the same signature, so any
# difference between them is a build-time bug, not a style nit.
set -eu

GO_SRC="cmd/update.go"
SH_SRC="install.sh"

extract_pem() {
    src="$1"
    awk '
        /-----BEGIN PUBLIC KEY-----/ {
            line = $0
            start = index(line, "-----BEGIN PUBLIC KEY-----")
            print substr(line, start)
            capturing = 1
            next
        }
        capturing && /-----END PUBLIC KEY-----/ {
            line = $0
            end = index(line, "-----END PUBLIC KEY-----") + length("-----END PUBLIC KEY-----") - 1
            print substr(line, 1, end)
            capturing = 0
            next
        }
        capturing { print }
    ' "$src"
}

go_key=$(extract_pem "$GO_SRC")
sh_key=$(extract_pem "$SH_SRC")

if [ -z "$go_key" ]; then
    echo "check-pubkey-sync: no PEM public key block found in $GO_SRC" >&2
    exit 1
fi

if [ -z "$sh_key" ]; then
    echo "check-pubkey-sync: no PEM public key block found in $SH_SRC" >&2
    exit 1
fi

if [ "$go_key" != "$sh_key" ]; then
    echo "check-pubkey-sync: release signing public key differs between $GO_SRC and $SH_SRC" >&2
    echo "--- $GO_SRC ---" >&2
    echo "$go_key" >&2
    echo "--- $SH_SRC ---" >&2
    echo "$sh_key" >&2
    exit 1
fi

echo "check-pubkey-sync: release signing public key matches in $GO_SRC and $SH_SRC"
