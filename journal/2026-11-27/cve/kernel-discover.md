# Kernel CVE ranges — 2026-11-27

Kernels: `5.10.134-013.15.kangaroo.al8.x86_64`, `6.1` (line), `6.6` (line), `6.12` (line)

> Upstream NVD ranges. Vendor kernels backport fixes without a version bump:
> 'affected' on a vendor kernel = check the vendor advisory, not proof of exposure.
> Sandbox exposure (capdrop/seccomp/netns) is triaged in docs/cve-watchlist.md.

| CVE | CVSS | Published | Origin | 5.10.134-013.15.kangaroo.al8.x86_64 | 6.1 | 6.6 | 6.12 | Note |
|---|---|---|---|---|---|---|---|---|
| CVE-2022-0847 | 7.8 HIGH | 2022-03-10 | watchlist | not affected | not affected | not affected | not affected | A flaw was found in the way the "flags" member of the new pipe buffer  |
| CVE-2022-0185 | 8.4 HIGH | 2022-02-11 | watchlist | not affected | not affected | not affected | not affected | A heap-based buffer overflow flaw was found in the way the legacy_pars |
| CVE-2023-0386 | 7.8 HIGH | 2023-03-22 | watchlist | not affected | fixed in 6.1.9 | not affected | not affected | A flaw was found in the Linux kernel, where unauthorized access to the |
| CVE-2024-1086 | 7.8 HIGH | 2024-01-31 | watchlist | affected (upstream range) | fixed in 6.1.76 | fixed in 6.6.15 | not affected | A use-after-free vulnerability in the Linux kernel's netfilter: nf_tab |
| CVE-2021-22555 | 8.3 HIGH | 2021-07-07 | watchlist | not affected | not affected | not affected | not affected | A heap out-of-bounds write affecting Linux since v2.6.19-rc1 was disco |
| CVE-2026-98432 | 9.8 CRITICAL | 2026-10-15 | watchlist | affected (upstream range) | fixed in 6.1.190 | fixed in 6.6.159 | fixed in 6.12.113 | In the Linux kernel, the following vulnerability has been resolved:  i |
| CVE-2026-31431 | 7.8 HIGH | 2026-04-22 | watchlist | affected (upstream range) | fixed in 6.1.170 | fixed in 6.6.137 | fixed in 6.12.85 | In the Linux kernel, the following vulnerability has been resolved:  c |
| CVE-2025-39964 | 7.8 HIGH | 2025-10-13 | watchlist | affected (upstream range) | fixed in 6.1.154 | fixed in 6.6.108 | fixed in 6.12.49 | In the Linux kernel, the following vulnerability has been resolved:  c |
| CVE-2026-53266 | 8.8 HIGH | 2026-06-25 | watchlist | affected (upstream range) | fixed in 6.1.176 | fixed in 6.6.143 | fixed in 6.12.94 | In the Linux kernel, the following vulnerability has been resolved:  n |
| CVE-2025-39682 | 9.8 CRITICAL | 2025-09-05 | watchlist | not affected | fixed in 6.1.149 | fixed in 6.6.103 | fixed in 6.12.44 | In the Linux kernel, the following vulnerability has been resolved:  t |

Discovery (last 2 d modified, <= 120 d published): CRITICAL 0 NVD hits, HIGH 21 NVD hits -> 0 listed; control: kernel-cpe-119d 7853, kernel-cpe-any-severity 55, kernel-text 68, all-products 2274

Note: 68 kernel CVE(s) changed in this window are listed by text; those NVD has not analysed yet (no CPE/CVSS) are evaluated on the day NVD adds their CPE.
