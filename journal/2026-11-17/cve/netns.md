# CVE check — 2026-11-17
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
| CVE-2026-31431 | kernel | CVSS: 7.8 HIGH | from cve-watchlist.md: 7.8 HIGH (NVD) | 6.18.22+/6.19.12+ (advisory) | |
| CVE-2025-39964 | kernel | CVSS: 7.8 HIGH | from cve-watchlist.md: 7.8 HIGH (NVD) | sau stable point | unverified  |
| CVE-2026-53266 | kernel | CVSS: 8.8 HIGH | from cve-watchlist.md: 8.8 HIGH (NVD) | sau stable point | unverified  |
| CVE-2025-39682 | kernel | CVSS: 9.8 CRITICAL | from cve-watchlist.md: **9.8 CRITICAL** (NVD) | sau stable point | unv |
| CVE-2026-124739 | busybox httpd | CVSS: 7.5 HIGH | from cve-watchlist.md: 7.5 HIGH | chưa công bố trong NVD refs | unveri |
| CVE-2026-88829 | busybox httpd | CVSS: 7.2 HIGH | from cve-watchlist.md: 7.2 HIGH | chưa công bố | unverified (published |
| CVE-2026-88837 | busybox httpd | CVSS: 6.5 MEDIUM | from cve-watchlist.md: 6.5 MEDIUM | chưa công bố | unverified | **khôn |
| CVE-2026-88831 | busybox httpd | CVSS: 5.3 MEDIUM | from cve-watchlist.md: 5.3 MEDIUM | chưa công bố | unverified | **khôn |
| CVE-2026-88834 | busybox httpd | CVSS: 3.7 LOW | from cve-watchlist.md: 3.7 LOW | chưa công bố | unverified | **không v |
