# WhatsApp Desk v1.5.7 — Benchmark macOS (Apple Silicon)
Tanggal: 11 Sep 2026 · Build: /Applications 1.5.7 universal · metode: ps rss/cpu sampling

## Fresh launch
- Spawn binary → jendela siap: <1s (proses shell), render web ~6s
- Proses: 1 shell (42 MB) + WebContent (~83-146 MB) + GPU (18 MB) + Networking (24-46 MB)
- TOTAL ~4 proses, 157-240 MB real memory setelah render

## Idle
- CPU idle stabil 1.8-4.4% (akumulasi 4 proses, mayoritas WebContent decoding/notif)
- MEM idle: 240 MB

## Minimize (purge path aktif)
- MEM turun 240 MB → 132 MB setelah minimize (purgeWebKitMemory bekerja)

## Drift (3 menit pemakaian normal)
- MEM: 234 → 803 (spike decoding media/chat) → 303 MB. Spike = decode media, bukan leak; kembali ~300 MB
- CPU spike 124% sesaat (decode), lalu 27% → normal 1-6%

## Cache disk
- ~/Library/Caches/com.whatsapp.desk: 173 MB (URL cache, kap 256 MB)
- ~/Library/WebKit/com.whatsapp.desk: 130 MB (NetworkCache WebKit, TIDAK dibatasi kode)

## Single instance
- Buka binary dobel → tetap 1 proses app (flock bekerja)
