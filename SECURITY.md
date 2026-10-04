# Security Policy

## Supported Versions

Security updates are currently provided for the latest version available on the
`main` branch.

| Version or Branch | Supported |
| ----------------- | --------- |
| `main`            | ✅ Yes     |
| Older releases    | ❌ No      |

Older versions may contain known security issues and are not guaranteed to
receive security updates.

## Reporting a Vulnerability

Please do not report security vulnerabilities through public GitHub issues.

If GitHub Private Vulnerability Reporting is enabled for this repository, please
use that feature to submit your report.

If private reporting is not available, contact the project maintainer privately
through the maintainer's GitHub profile before disclosing any vulnerability
details publicly.

Please include:

- A clear description of the vulnerability.
- The affected version, branch, or commit.
- Steps to reproduce the issue.
- The potential impact.
- Proof-of-concept code or screenshots, if applicable.
- Any suggested mitigation or fix.

Please remove or redact passwords, tokens, phone numbers, chat content, personal
data, and other sensitive information before submitting a report.

## Disclosure Policy

Please allow the maintainers reasonable time to investigate and address the
issue before making vulnerability details public.

The maintainers may:

- Acknowledge the report.
- Request additional information.
- Confirm or reject the vulnerability.
- Prepare and release a fix.
- Credit the reporter if they agree.

Please do not publicly disclose the vulnerability before coordinating with the
maintainers.

## Antivirus False Positives

Windows Defender and other engines occasionally flag the Windows installer as
`Trojan:Win32/Wacatac.B!ml` or similar. These are **false positives**. The
releases are unsigned community builds, and the bundled Go and WebView2 binaries
trip heuristic detection that has nothing to do with the application code. The
`!ml` suffix marks a machine-learning guess rather than a signature match, which
is why the same file is reported by one engine and not another.

Before treating a detection as real:

1. **Verify the download.** Compare the file's SHA-256 against the `SHA256SUMS`
   asset published with the same release:

   ```
   certutil -hashfile WhatsApp-Desk-Windows-x64-Setup.exe SHA256
   ```

   A mismatch is a genuine problem — do not run the file. A match means you have
   the exact bytes the project published.
2. **Build it yourself** if you would rather not trust the binary:
   `./build_windows.sh` produces the same installer from source.
3. **Report the false positive to your vendor.** Microsoft accepts submissions at
   <https://www.microsoft.com/en-us/wdsi/filesubmission> and usually clears them
   within a day. Doing this is what stops the next person from hitting it.

Every detection reported so far has been cleared as a false positive. No release
has ever contained code that contacts anything beyond `https://web.whatsapp.com`
for chat traffic and this repository's own GitHub releases for updates.

## Scope

This policy covers security issues in the WhatsApp Desktop application and its
source code.

Issues caused by WhatsApp's servers, WhatsApp accounts, operating systems, or
third-party dependencies may need to be reported to the relevant vendor.
