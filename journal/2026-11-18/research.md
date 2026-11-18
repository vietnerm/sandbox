# Research 2026-11-18 - Unicode ẩn / bidi (Trojan Source) trong CI

## Phạm vi
Kiểm chứng các tool trong bản "chiến lược CI/CD 3 lớp" owner dán vào (do AI khác
viết) trước khi dùng cho PR #36. Kết luận dùng ở `decisions/unicode-guard.md` (D29-D30; ghi D28-D29 trên nhánh trước khi sync).

## Mối đe doạ
- **Trojan Source, CVE-2021-42574** (Boucher & Anderson, 2021, trojansource.codes):
  ký tự điều khiển bidi (RLO U+202E, LRI/RLI/FSI/PDI U+2066-2069...) làm thứ tự
  hiển thị khác thứ tự compiler đọc. CVE-2021-42694: homoglyph trong định danh.
- **ASCII smuggling** qua khối tag U+E0000-E007F và variation selector
  (U+FE00-FE0F, U+E0100-E01EF): payload vô hình trong văn bản, nhắm cả LLM/agent đọc repo.
- Unicode TR#9 (Bidirectional Algorithm) và TR#36/39 (Security Considerations /
  Mechanisms): danh mục control và confusable dùng để chọn tập ký tự error/warn.

## Kiểm chứng tool được nêu (2026-11-18)

| Tool | Nêu trong bản gốc | Kiểm được | Dùng? |
|---|---|---|---|
| GlyphGuard (`NicPWNs/GlyphGuard`) | GitHub Action "chính thức" | repo có thật, tag v0.2.0, v0.3.0 | Không: dự án nhỏ 1 người, v0.x |
| `heckler` | Python CLI `--ci` | PyPI 2.0.0, repo `kholcomb/heckler` | Không: bản gốc `pip install` không ghim |
| `anti-trojan-source` | "từ ortoo" | npm 1.13.1 nhưng của **lirantal**, không phải ortoo | Không: cần Node, nguồn bị nêu sai |
| `out-of-character` | CLI dọn ký tự | npm 2.3.0 (spencermountain); **không** có trên PyPI | Không: cần Node |
| `eslint-plugin-anti-trojan-source` | IDE | chỉ JS/TS; repo này C/Go/Python/shell | Không áp dụng |
| `cleaner-code` (CodeSafer) | dọn ký tự | không kiểm chứng được | Không |

=> Tự viết scanner stdlib (~260 dòng). Lợi: không phụ thuộc, không mạng, đọc hết
được trong 1 lần review; hiểu tiếng Việt (không coi dấu là lỗi).

## Ghi chú môi trường
- `rootfs-alpine/` có ~335 symlink busybox: scanner bỏ qua symlink, không đi theo
  ra ngoài cây.
- Repo tại 81b5dce: 204 file text, 0 error, 0 warning.

---


# Research — 2026-11-18 (phiên orchestrator 2, chủ đề: Trojan Source / Unicode guard)

> Liên quan PR #36 (ci/unicode-guard). Mục đích: đủ cơ sở đánh giá scanner tự viết
> của PR so với state of the art. Nguồn đã mở/verify link thật.

## Nguồn 1 — NVD CVE-2021-42574 (Trojan Source, bidi)
- Link: https://nvd.nist.gov/vuln/detail/CVE-2021-42574 (đã verify qua search NIST)
- Tóm tắt: lỗ hổng theo dõi công khai cho tấn công tái sắp xếp hiển thị qua thuật toán
  bidirectional của Unicode ≤ 14.0 — code hiển thị với reviewer khác với thứ tự parser
  đọc. Miễn phí giấy phép special-exception, điển hình: chèn RLO/PDI vào comment/string
  để ẩn nhánh `if (admin)`. Ảnh hưởng compiler/language đa số; mitigation chính là
  tooling quét ở review/CI, không phải vá compiler.
- Áp dụng cho minict: repo c code + shell + workflow YAML — một ký tự RLO trong
  `sandbox/minict.c` đủ đổi ngữ nghĩa lệnh mount/unshare cho người review. Lớp PR-gate
  của PR #36 chặn đúng lớp này trước khi vào main.

## Nguồn 2 — Paper gốc: "Trojan Source: Invisible Vulnerabilities" (Boucher & Anderson)
- Link: https://arxiv.org/abs/2111.00169 (verify qua search), mã nguồn PoC:
  https://github.com/nickboucher/trojan-source
- Tóm tắt: hệ thống hoá các attack class: Bidi_override, early_return (RLO trong
  string), homoglyph_attack (đổi identifier bằng ký tự giống mắt — scanner trả về
  warn, UG8), Combining-character, bad_indent với mixed LTR/RTL. Đề xuất compiler
  warn + linter reject bidi/invisible.
- Áp dụng: bảng codepoint của unicode-guard.py (bidi U+202A–2069, invisible, tag
  U+E0000–E007F, VS supplement U+E0100–E01EF) phủ đúng các class trong paper; điểm
  paper không xử lý mà PR xử lý thêm: allowlist ngữ cảnh emoji (ZWJ/VS16) để không
  phá text thực — trade-off chấp nhận được vì emoji không xuất hiện trong code path.

## Nguồn 3 — ASCII smuggling qua Unicode Tags (vector mới 2024–2026)
- Link: https://embracethered.com/blog/posts/2024/hiding-and-finding-text-with-unicode-tags
  (verify) ; Microsoft Security Blog 2026-09-03:
  https://www.microsoft.com/en-us/security/blog/2026/09/03/ascii-smuggling-crosses-over-from-ai-prompt-injection-to-phishing-evasion (verify)
- Tóm tắt: block Tags U+E0000–E007F ánh xạ ASCII (U+E0041='A') render vô hình — dùng
  tráo instruction cho LLM/agent, hoặc giấu payload qua email/phishing (MS 2026).
  Không nằm trong paper 2021 → scanner chỉ check bidi thuần sẽ bỏ sót.
- Áp dụng: repo này có agent (ZAI/SCR/SIA/SBG) đọc prompt/journal — một ký tự tag
  nhét vào PROMPT.MD là vector prompt-injection. unicode-guard.py đánh dấu error
  (U+E0000–E007F + VS supplement) — quyết định D28 đúng hướng; UG3/UG10 test phủ.

## Trade-off tổng (ghi decision D28)
- Tự viết stdlib vs pip tool: mất vào-độ-mới, đổi lấy zero-supply-chain + audit
  được trong 263 dòng. Chấp nhận.
- warn không fail (NBSP/homoglyph): giảm false positive tiếng Việt; --strict cho
  ai muốn chặt. Chấp nhận.

---

# Research — 2026-11-18 (phiên orchestrator 3, chủ đề: S1 webproxy — symlink to unix socket, CWE-59)

> Cho issue #41 (`sec/s1-webproxy-symlink`). Câu hỏi: host connect tới unix socket
> nằm trên disk mà payload ghi được — có pattern kháng symlink + TOCTOU chuẩn nào?
> Nguồn đã mở/verify qua search.

## Nguồn 1 — CWE-59: Improper Link Resolution Before File Access ("Link Following")
- Link: https://cwe.mitre.org/data/definitions/59.html (verify)
- Tóm tắt: mở file theo tên nhưng tên bị đổi thành link giữa lúc check và lúc dùng
  (TOCTOU) hoặc code follow link không kiểm — hậu quả ghi/đọc ngoài dự kiến, leo
  thang đặc quyền. CWE nhấn mạnh mitigation hợp lệ là mở bằng fd (openat/O_NOFOLLOW),
  còn lstat-then-use chỉ giảm cửa sổ, không xoá.
- Áp dụng minict: webproxy.py dùng lstat/connect(path) đúng pattern CWE-59 — phải
  chuyển sang resolve-via-fd, không chỉ thêm lstat check (lstat vẫn còn TOCTOU).

## Nguồn 2 — CVE-2018-15664: docker cp symlink-race (phân tích Aleksa Sarai/cyphar)
- Link: phân tích "Docker is vulnerable to a symlink-race attack" (cyphar, 05/2019;
  discuss qua search: "O_PATH can definitely help, especially if you actually check
  that the path is what you think through readlink(/proc/self/fd/$n)") (verify qua search)
- Tóm tắt: docker cp chexec từ container nhét symlink vào path trong lúc TarOptions
  check → daemon host-side follow ra host fs. Mitigation trong runc/docker sau này:
  open toàn bộ path bằng O_PATH|O_NOFOLLOW từng thành phần rồi dùng fd, verify inode
  qua /proc/self/fd — đúng hướng "không resolve path lần 2".
- Áp dụng minict: tình huống S1 là dạng con của CVE này (host daemon resolve path
  điều khiển được bởi payload). Kế thừa pattern O_PATH từng thành phần; khác biệt:
  ta cần CONNECT chứ không mở file → nối qua /proc/self/fd (nguồn 3).

## Nguồn 3 — Pattern `O_PATH` + `connect("/proc/self/fd/<fd>")` cho AF_UNIX
- Link: thảo luận kernel/linux-distros về unix socket path length + O_PATH
  ("when connect()ing you first open the socket inode with O_PATH, and then you
  fire the connect() specifying /proc/self/fd/<fd> as path"; tương tự fexecve
  trước execveat) (verify qua search, people.kernel.org thread)
- Tóm tắt: connect() nhận path ≤107 bytes và resolve lại path — nhưng nếu path là
  magic link /proc/self/fd/<n> trỏ tới O_PATH fd đã mở, kernel resolve thẳng tới
  inode đã pin: vừa né giới hạn 107 bytes, vừa kháng symlink/TOCTOU vì không có
  path-walk lại. Dùng phổ biến trong systemd (socket activation) và runtime
  container.
- Áp dụng minict: đây là mảnh cuối cùng của fix — resolver trả về fd đã verify
  S_ISSOCK, connect qua magic link. WEB9 positive control chứng minh pattern chạy
  được trên kernel 5.10 dev machine (và sẽ được CI runner azure xác nhận lần nữa).

## Nguồn 4 — CVE-2026-7374 (OpenShift): symlink validation → truy cập unix socket host
- Link: https://nvd.nist.gov/vuln/detail/CVE-2026-7374 (verify qua search NVD, 05/2026)
- Tóm tắt: user authenticated validated symlink không đúng cách → truy cập BẤT KỲ
  unix socket nào trên host (CWE-59) — đây chính là primitive tấn công của S1 nếu
  không fix: payload trong sandbox → cầu host proxy → socket host tuỳ ý cùng uid.
- Áp dụng minict: xác nhận mức độ đe dọa là thực tế và hiện đại (CVSS cao, 2026),
  không phải threat-model lý thuyết; củng cố việc chọn priority:p1 cho S1.
