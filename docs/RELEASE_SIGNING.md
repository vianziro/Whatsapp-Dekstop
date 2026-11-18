# Release signing

Signing is optional for local development builds. Public release builds should
be signed with organization-controlled certificates; never commit a certificate
or password to this repository.

## Windows Authenticode

The Windows build scripts sign both `WhatsAppDesk.exe` and the NSIS installer
when all of the following are available:

- `signtool` is on `PATH` (Windows SDK)
- `WA_WINDOWS_SIGN_CERT` points to a PFX certificate file
- `WA_WINDOWS_SIGN_PASSWORD` contains its password

The scripts timestamp with SHA-256 using DigiCert's timestamp service by
default. Override it with `WA_WINDOWS_SIGN_TIMESTAMP_URL` if your certificate
provider specifies another RFC 3161 timestamp URL. Every signed artifact is
verified with `signtool verify /pa` before packaging completes.

For the manual GitHub Release workflow, configure these Actions secrets:

- `WINDOWS_SIGNING_CERT_BASE64`: base64-encoded PFX file
- `WINDOWS_SIGNING_CERT_PASSWORD`: PFX password

The workflow writes the certificate only to the ephemeral runner's temporary
directory. If the secrets are absent, builds remain unsigned. Authenticode
signing improves publisher attribution and SmartScreen reputation over time;
it does not guarantee that every antivirus heuristic will accept a new build.
Submit any remaining Microsoft Defender detection through Microsoft's malware
submission portal and include the release version and SHA-256 digest.

## macOS stable local identity (no paid certificate)

Ad-hoc signatures (`codesign -s -`) change the app's designated requirement on
every build. macOS ties the keychain item WebKit keeps the WebCrypto master key
in — the key that encrypts the WhatsApp session in IndexedDB — to that
requirement, so every rebuild or update of an ad-hoc build asks for the login
password again, and *Deny* makes WhatsApp report "A database error occurred …
relink your device" (#74).

Run once per build machine:

```bash
bash make_mac_signing_identity.sh
```

It creates a self-signed code-signing identity, "WhatsApp Desk Local Signing",
in the login keychain. `build_mac.sh` uses it automatically when
`WA_MAC_SIGN_IDENTITY` is not set (override the name with
`WA_MAC_LOCAL_SIGN_NAME`, or force ad hoc with `WA_MAC_ADHOC=1`). The resulting
requirement is certificate-based and identical across builds, so *Always Allow*
only has to be clicked once. It is not a Developer ID: Gatekeeper still treats
the app as from an unidentified developer and it cannot be notarized.

If you publish builds signed with this identity, export it from Keychain Access
(`.p12`) and keep it safe: a new identity means one more keychain prompt for
every user.

## macOS Developer ID and notarization

For a local release, install the Developer ID Application certificate in the
login keychain and set:

```bash
export WA_MAC_SIGN_IDENTITY='Developer ID Application: Your Organization (TEAMID)'
```

The build signs with hardened runtime and verifies the bundle. To notarize,
first store credentials in the keychain (once per release machine):

```bash
xcrun notarytool store-credentials WhatsAppDeskNotary \
  --apple-id 'maintainer@example.com' \
  --team-id 'TEAMID' \
  --password 'app-specific-password'
export WA_MAC_NOTARY_PROFILE=WhatsAppDeskNotary
```

The macOS build submits a temporary ZIP, waits for Apple's result, staples the
ticket to the app bundle, and validates the staple before creating the DMG/ZIP
artifacts. Without `WA_MAC_SIGN_IDENTITY`, it makes the existing ad-hoc signed
development build. A Developer ID signature without a notarization profile is
signed but not notarized.

The manual macOS workflow accepts these repository Actions secrets:

- `MACOS_SIGNING_CERT_BASE64`: base64-encoded Developer ID P12
- `MACOS_SIGNING_CERT_PASSWORD`: P12 password
- `MACOS_SIGNING_IDENTITY`: exact certificate identity string
- `APPLE_ID`, `APPLE_TEAM_ID`, `APPLE_APP_SPECIFIC_PASSWORD`: notarization
  credentials

Notarization is attempted only when a signing certificate and all three Apple
credentials are present. Credentials and temporary certificate material should
be rotated/revoked immediately if exposed.
