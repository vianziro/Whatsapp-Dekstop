# macOS microphone diagnosis (#57)

The audio issue is reported on macOS while Safari and Chrome work on the same
machine. WhatsApp Desk currently presents WKWebView to WhatsApp using a Chrome
compatibility user agent. Since the report's diagnostic log has not yet
captured a `getUserMedia` call, changing capture constraints would be guesswork.

This build adds a reversible A/B test for the user-agent hypothesis. The normal
launch is unchanged. After building a version containing this change, run a
normal test call first with diagnostics enabled:

```bash
WA_DESK_DEBUG=1 "/Applications/WhatsApp Desk.app/Contents/MacOS/WhatsApp"
```

Then quit and repeat the same call with Safari identity enabled:

```bash
WA_DESK_DEBUG=1 WA_DESK_SAFARI_UA_TEST=1 \
  "/Applications/WhatsApp Desk.app/Contents/MacOS/WhatsApp"
```

Send the `mic:` lines from
`~/Library/Application Support/WhatsAppDesk/wa_debug.log` and report whether
WhatsApp accepted the session and whether outgoing audio remained robotic.
The diagnostic mode may be rejected by WhatsApp; that result is useful too.
It changes only the user-agent identity and does not override audio constraints.
