# D56 — GĐ2 mục 2: template (save instance as template, run from template) (issue #127, Phiên 21, 2026-11-23)

## Ngữ cảnh

Manifest GĐ2 mục 2 DoD: "Template: tạo instance từ template; lưu instance thành template."
Hiện `prepareRootfs` hỗ trợ base/dir/tar/oci/path — không có cách save một instance thành
template tái sử dụng được. Mỗi instance copy rootfs từ base (2 MB/instance) — không share.

## Quyết định

Chọn **template = named rootfs snapshot trong `<Root>/_templates/<name>/`**, không overlayfs:

1. **`template:<name>` image form** (`rootfs.go prepareRootfs`): case mới → copy
   `<Root>/_templates/<name>/rootfs` → instance rootfs (cp -a, parity `base`/`dir:`).
   `validName` check trên template name (path injection defense).
2. **`SaveTemplate(name, templateName)`** (`runtime.go`): yêu cầu instance STOPPED
   (fail-closed `ErrInstanceRunning` → 409 — copy running rootfs races payload writes,
   inconsistent snapshot). `copyTree` rootfs + spec.json vào `<Root>/_templates/<name>/`.
3. **`ErrInstanceRunning` sentinel** → `mapErr` 409 Conflict (parity `ErrPortInUse`/
   `ErrNotRunning` — "state conflict, op requires stopped").
4. **`ListByNamePrefix` filter `nameRe`** (skip `_templates` + non-instance dirs) —
   không pollute List API. `_` prefix reserved cho internal dirs.
5. **API**: `POST /v1/sandboxes/{name}/save-template` body `{"template":"<name>"}` → 200.
6. **Không overlayfs**: template = full rootfs copy (cp -a, ~2 MB/template). Overlayfs
   rootless + idmapped mounts là mục 3 (storage), riêng biệt. Template scope = frozen
   base tái sử dụng, không thin-provision.

## Phủ nhận / residual

- **List/remove-template API**: chưa có. Cleanup qua state dir (`rm _templates/<name>`)
  hoặc qua test WORK dir. Add API sau nếu cần (low priority — operator có thể rm dir).
- **Template spec.json**: lưu reference (copy spec.json vào template), nhưng Run từ
  template vẫn yêu cầu operator cung cấp full RunSpec (cmd/env/etc). Chưa auto-apply
  template defaults (vd. "run from template, inherit cmd"). Residual: API "clone
  template with defaults" sau.
- **Running instance live snapshot**: không hỗ trợ (fail-closed 409). Docker cho phép
  `docker commit` trên running container — minict yêu cầu stop trước (safer: consistent
  rootfs). Residual: nếu cần live snapshot, dùng overlayfs freeze (mục 3).
- **Disk**: mỗi template ~2 MB (full rootfs copy). Không thin-provision (mục 3 overlayfs).

## Tests

- **MD39**: run src (base, write `/tpl-marker`), save-template on RUNNING → 409; stop src;
  save-template `md-tpl` → 200; run dst `image=template:md-tpl` → exec `cat /tpl-marker` ==
  "template-marker" (proves rootfs cloned from template). Marker ở `/` (persistent rootfs),
  KHÔNG `/tmp` (tmpfs — sẽ không copy ra host rootfs dir).
- Mutant (SCR): skip `validName` check → path traversal `template:../x` → red; skip stopped
  check → running save-template succeeds → MD39 409 assertion red.

## Bài học đo được

- **`/tmp` là tmpfs**: marker phải ở rootfs persistent (`/`), không `/tmp` (tmpfs in-memory,
  không copy ra host rootfs dir). Đo được khi thiết kế MD39.
- **`_` prefix reserved**: ListByNamePrefix filter nameRe (`^[a-zA-Z0-9]`) → `_templates`
  tự skip. Convention: internal dirs dùng `_` prefix.
- **cp -a parity**: template copy dùng cùng `copyTree` (cp -a) như base/dir — consistent
  symlink/mode/ownership semantics, không thêm extractor path mới.
