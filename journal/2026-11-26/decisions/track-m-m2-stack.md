# Decision D75 — Track M M2: stack.json schema + sandbox stack up|down|ps

- **Phiên**: 39 (2026-11-26)
- **Issue**: #199
- **Slug**: `track-m-m2-stack`

## Vấn đề
Track M M2 (roadmap dòng 714-722): công cụ `sandbox stack up|down|ps` cho multi-instance stacks (vd. web + db) với phụ thuộc + chờ ready. Daemon không đổi — parser + CLI ở lớp client.

## Giải pháp chọn

### Format: JSON (stack.json)
- **Không YAML**: go.mod hiện zero external deps (stdlib only). Thêm gopkg.in/yaml.v3 (~100KB) là dep đầu tiên → vi phạm manifest §3 "không thêm phụ thuộc" (Track M). JSON: stdlib `encoding/json`, zero dep.
- User muốn YAML → `yq -ojson stack.yaml > stack.json` (công cụ có sẵn, manifest §3 "không phát minh lại").
- File name: `stack.json` (có thể override qua `--file`).

### Schema
```json
{
  "name": "myapp",
  "services": {
    "web": {
      "image": "base",
      "cmd": ["/bin/sh", "-c", "httpd -f -p 80 ..."],
      "env": ["FOO=bar"],
      "cpus": "0.5",
      "memory": "128m",
      "pids_limit": 100,
      "mounts": ["/host/data:/data:ro"],
      "labels": ["minict.webport=8080"],
      "depends_on": ["db"],
      "ready": {
        "type": "http",
        "port": 8080,
        "path": "/",
        "timeout_s": 30
      }
    },
    "db": {
      "image": "template:alpine-sqlite",
      "cmd": ["/bin/sh", "-c", "sqlite3 /data/app.db ..."],
      "mounts": ["/host/db:/data"]
    }
  }
}
```

### Trường lạ → lỗi
`json.Decoder.DisallowUnknownFields()` — trường không trong schema → parse error, không bỏ qua im lặng (roadmap yêu cầu).

### depends_on + ready
- `depends_on`: list of service names. `stack up` starts dependencies first (topological sort).
- `ready`: check after start. Types:
  - `http`: GET http://127.0.0.1:{port}{path} → 2xx = ready.
  - `tcp`: connect 127.0.0.1:{port} → success = ready.
  - `exec`: run `sandbox exec {name} {cmd}` → exit 0 = ready.
  - `none` (default): no wait, start immediately.
- `timeout_s`: default 30. Exceed → error, `stack down` cleanup.

### CLI
```
sandbox stack up [--file stack.json]   # start all services (depends_on order)
sandbox stack down [--file stack.json]  # stop + remove all
sandbox stack ps [--file stack.json]    # list status
```

### Hai phép thử manifest §3
1. **Còn nhẹ không?** Client-side only — `sandbox` binary tăng ~20KB (parser + CLI code). Daemon 0 impact (gọi API như user). Stdlib json, zero dep.
2. **Bỏ đi core còn hữu dụng?** Có — core (minictd API) hoạt động độc lập. Stack là tiện ích multi-instance.

## Không làm
- YAML parser (stdlib only — JSON đủ; user dùng `yq` nếu cần).
- docker-compose compatibility (manifest §4 non-goal).
- Watch/restart policy (ngoài `depends_on` + `ready`).
- Environment variable interpolation (làm sau nếu cần).
