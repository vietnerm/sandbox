# CVE check — 2026-11-19
- kernel requested: `6.18.44-fc-v653`; host: `6.18.44-fc-v653`

| CVE | Component | CVSS | Why tracked |
|---|---|---|---|
| CVE-2022-0847 | kernel(5.10) | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | Dirty Pipe — pipe buffer flags; affects 5.8+; fixed mainline 5.10.102 |
| CVE-2022-0185 | kernel(5.10) | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | fs context heap overflow — container escape vector; fixed 5.10.87 |
| CVE-2023-0386 | kernel(5.10) | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | overlayfs userns copy-up setuid exec; fixed 5.10.179 — we AVOID overla |
| CVE-2024-1086 | kernel(5.10) | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | nf_tables UAF LPE — requires CAP_NET_ADMIN; blocked by our capdrop (me |
| CVE-2021-22555 | kernel(5.10) | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | netfilter x_tables heap OOB; fixed 5.10.42 |
| CVE-2024-21626 | runc(engine) | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | Leaky Vessels fd leak — N/A for us (no OCI runtime) but tracked as eng |
| CVE-2026-98432 | kernel | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: **9.8 CRITICAL** (NVD) | mainline 11/2026 (sau  |
| CVE-2026-31431 | kernel | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: 7.8 HIGH (NVD) | 6.18.22+/6.19.12+ (advisory) | |
| CVE-2025-39964 | kernel | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: 7.8 HIGH (NVD) | sau stable point | unverified  |
| CVE-2026-53266 | kernel | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: 8.8 HIGH (NVD) | sau stable point | unverified  |
| CVE-2025-39682 | kernel | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: **9.8 CRITICAL** (NVD) | sau stable point | unv |
| CVE-2026-124739 | busybox httpd | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: 7.5 HIGH | chưa công bố trong NVD refs | unveri |
| CVE-2026-88829 | busybox httpd | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: 7.2 HIGH | chưa công bố | unverified (published |
| CVE-2026-88837 | busybox httpd | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: 6.5 MEDIUM | chưa công bố | unverified | **khôn |
| CVE-2026-88831 | busybox httpd | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: 5.3 MEDIUM | chưa công bố | unverified | **khôn |
| CVE-2026-88834 | busybox httpd | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: 3.7 LOW | chưa công bố | unverified | **không v |
| CVE-2021-30139 | apk-tools (tarball parsi | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: 7.0 HIGH | 2.12.0 | **patched** (2.14.13 ≫ 2.12 |
| CVE-2021-36159 | apk-tools (OOB read, HTT | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: 9.8 CRITICAL | 2.12.6 | **patched** (2.14.13 ≫  |
| CVE-2026-39973 | **Apktool (Android RE to | CVSS: ERR (<urlopen error Tunnel connection failed: 403 Forbidden (agent proxy: no rule or ) | from cve-watchlist.md: 7.5 | — | **N/A — component khác, chỉ trùng tên |
saved: journal/2026-11-19/cve/proc-pidns.json
