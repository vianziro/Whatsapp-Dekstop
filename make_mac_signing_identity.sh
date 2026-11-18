#!/usr/bin/env bash
# Creates a stable, self-signed code-signing identity for WhatsApp Desk builds
# in your login keychain. Run it ONCE per build machine; build_mac.sh picks it
# up automatically.
#
# Why: WebKit keeps the key that encrypts the WhatsApp session (IndexedDB) in
# a keychain item named "WhatsApp Desk WebCrypto Master Key". macOS grants an
# app access to that item by the app's code-signing requirement. An ad-hoc
# signature produces a different requirement on every build, so after every
# rebuild/update macOS asks for your login password again - and "Deny" makes
# WhatsApp report "A database error occurred ... relink your device" (#74).
# A certificate-based signature keeps the same requirement across builds, so
# clicking "Always Allow" once is enough.
#
# This is NOT a Developer ID: Gatekeeper still treats the app as from an
# unidentified developer (same as ad-hoc). Back up the identity (Keychain
# Access -> export .p12) if you publish builds signed with it; a new identity
# means one more keychain prompt for every user.
set -euo pipefail

NAME="${WA_MAC_LOCAL_SIGN_NAME:-WhatsApp Desk Local Signing}"
KEYCHAIN="${WA_MAC_LOCAL_SIGN_KEYCHAIN:-$HOME/Library/Keychains/login.keychain-db}"

if [ "$(uname -s)" != "Darwin" ]; then
    echo "This script only runs on macOS." >&2
    exit 1
fi
if security find-certificate -c "${NAME}" >/dev/null 2>&1; then
    echo "Identity '${NAME}' already exists; nothing to do."
    exit 0
fi

# Apple's /usr/bin/openssl (LibreSSL) writes PKCS#12 that `security import`
# reads. OpenSSL 3 (e.g. Homebrew) needs -legacy for that.
OPENSSL=/usr/bin/openssl
[ -x "${OPENSSL}" ] || OPENSSL="$(command -v openssl)"
LEGACY=""
if "${OPENSSL}" pkcs12 -help 2>&1 | grep -q -- '-legacy'; then
    LEGACY="-legacy"
fi

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

cat > "${TMP}/cert.cnf" <<CNF
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = ${NAME}
[ext]
basicConstraints = critical, CA:false
keyUsage = critical, digitalSignature
extendedKeyUsage = critical, codeSigning
subjectKeyIdentifier = hash
CNF

"${OPENSSL}" req -x509 -newkey rsa:2048 -nodes -days 3650 -sha256 \
    -keyout "${TMP}/key.pem" -out "${TMP}/cert.pem" -config "${TMP}/cert.cnf" >/dev/null 2>&1

P12_PASS="$("${OPENSSL}" rand -hex 16)"
# shellcheck disable=SC2086
"${OPENSSL}" pkcs12 -export ${LEGACY} -inkey "${TMP}/key.pem" -in "${TMP}/cert.pem" \
    -name "${NAME}" -out "${TMP}/identity.p12" -passout "pass:${P12_PASS}"

# -T /usr/bin/codesign lets codesign use the private key without a prompt.
security import "${TMP}/identity.p12" -k "${KEYCHAIN}" -P "${P12_PASS}" -T /usr/bin/codesign >/dev/null

if ! security find-certificate -c "${NAME}" >/dev/null 2>&1; then
    echo "Import failed: '${NAME}' is not in ${KEYCHAIN}." >&2
    exit 1
fi

echo "Created '${NAME}' in ${KEYCHAIN}."
echo
echo "Next:"
echo "  1. bash build_mac.sh                      (signs with this identity automatically)"
echo "  2. Install the new app, launch it, and at the 'WebCrypto Master Key'"
echo "     prompt enter your login password and click 'Always Allow' - once."
echo "     Later builds signed with this identity will not ask again."
