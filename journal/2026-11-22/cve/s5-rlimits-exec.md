# CVE check — 2026-11-22
- kernel requested: `5.10.134-013.15.kangaroo.al8.x86_64`; host: `5.10.134-013.15.kangaroo.al8.x86_64`

| CVE | Component | CVSS | Why tracked |
|---|---|---|---|
| CVE-2022-0847 | kernel(5.10) | CVSS: 7.8 HIGH | Dirty Pipe — pipe buffer flags; affects 5.8+; fixed mainline 5.10.102 |
| CVE-2022-0185 | kernel(5.10) | CVSS: 8.4 HIGH | fs context heap overflow — container escape vector; fixed 5.10.87 |
| CVE-2023-0386 | kernel(5.10) | CVSS: 7.8 HIGH | overlayfs userns copy-up setuid exec; fixed 5.10.179 — we AVOID overla |
| CVE-2024-1086 | kernel(5.10) | CVSS: 7.8 HIGH | nf_tables UAF LPE — requires CAP_NET_ADMIN; blocked by our capdrop (me |
| CVE-2021-22555 | kernel(5.10) | CVSS: 8.3 HIGH | netfilter x_tables heap OOB; fixed 5.10.42 |
| CVE-2024-21626 | runc(engine) | CVSS: 8.6 HIGH | Leaky Vessels fd leak — N/A for us (no OCI runtime) but tracked as eng |
| CVE-2026-98432 | kernel | CVSS: 9.8 CRITICAL | from cve-watchlist.md: **9.8 CRITICAL** (NVD) | mainline 11/2026 (sau  |
