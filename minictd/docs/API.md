# minictd HTTP API

Mirror of `contract.Runtime` — one endpoint per op. JSON unless noted.
Auth: `Authorization: Bearer <token>` — **mandatory** (token from `$MINICTD_TOKEN` or `--token-file`; `--insecure-no-auth` only for local dev). Instance names in the URL must match `[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}` (else 400). Request bodies are capped at 16 MB.

| # | Op | Endpoint | In | Out / Errors |
|---|---|---|---|---|
| 1 | Run | `POST /v1/run` | `RunSpec` | `200 {"id":"s-..."}` · 400 validation · 500 runtime |
| 2 | Inspect | `GET /v1/sandboxes/{name}` | — | `200 ContainerJSON` · **404 ErrNotFound** |
| 3 | Remove | `POST /v1/sandboxes/{name}/remove` | — | `204` (idempotent: missing ⇒ 204) |
| 4 | Start | `POST /v1/sandboxes/{name}/start` | — | `204` · 404 · 500 (e.g. bind source gone) |
| 5 | Stop | `POST /v1/sandboxes/{name}/stop` | `{"timeout_sec":10}` (optional, default 10) | `204` · 404 |
| 6 | Exec | `POST /v1/sandboxes/{name}/exec` | `{"cmd":["..."]}` | `200 ExecResult` · 404 · **409 not running** |
| 7 | ExecV2 | `POST /v1/sandboxes/{name}/execv2` | `ExecV2Request` | `200 ExecV2Result` · 404 · 409 |
| 8 | ExecV2Stream | `POST /v1/sandboxes/{name}/execstream` | `ExecV2Request` | NDJSON stream (below) · 404 · 409 |
| 9 | Logs | `GET /v1/sandboxes/{name}/logs?tail=N&follow=1` | — | text/plain stream · 404 |
| 10 | ListByNamePrefix | `GET /v1/sandboxes?prefix=s-` | — | `200 {"names":[...]}` |
| 11 | Stats | `GET /v1/sandboxes/{name}/stats` | — | `200 Stats` · 404 |
| 12 | ServerVersion | `GET /v1/version` | — | `200 {"version":"..."}` |
| 13 | Info | `GET /v1/info` | — | `200 {"version":"..."}` |
| 14 | NetworkSubnets | `GET /v1/subnets?network=x` | — | `200 {"subnets":[...],"gateways":[...]}` |
| — | healthz | `GET /healthz` | — | `200 {"ok":"1"}` |
| — | RawState (extra) | `GET /v1/sandboxes/{name}/state` | — | `200 InstanceState` (notes, proxy pids, exec mode) · 404 |

Status codes: `404` = `contract.ErrNotFound`; `409` = exec on a non-running
instance; `400` = validation; `401` = bad token; `500` = the rest.

## ExecV2Request (JSON)

```json
{
  "cmd": ["/bin/sh", "-c", "make test"],
  "work_dir": "/workspace",
  "env": ["GREETING=bonjour"],
  "stdin": "aGVsbG8K",
  "timeout_s": 30,
  "max_output_bytes": 1000000
}
```

`stdin` is base64 (Go `[]byte` JSON encoding). `timeout_s` arms the
daemon watchdog: TERM to the exec process group, SIGKILL after 2s, result
`{"exit_code":124,"timed_out":true,...}`.

## ExecV2Stream wire format

`application/x-ndjson`, one object per line (see `contract/stream.go`):

```
{"t":"stdout","d":"b3V0MQo="}
{"t":"stderr","d":"ZXJyMQo="}
{"t":"exit","exit_code":0,"timed_out":false,"truncated":false,"duration_ms":12}
```

stdout/stderr frames interleave; exactly one `exit` frame terminates the
stream. The HTTP client re-materializes this into two `io.ReadCloser`s +
`wait()` — the exact `ExecV2Stream` signature of docker.Client.

## curl examples

```bash
curl -s -X POST localhost:7601/v1/run -d '{
  "name":"s-demo01", "image":"base", "hostname":"s-demo01",
  "cmd":["/bin/sh","-c","while true; do echo tick; sleep 1; done"],
  "env":["GREETING=bonjour"],
  "volumes":["/srv/work:/workspace:ro"],
  "tmpfs":["/tmp:size=512m"], "shm_size":"1g",
  "ulimits":["nofile=65536:65536"], "pids_limit":1024,
  "labels":["minict.webport=8125","traefik.enable=true"]
}'

curl -s localhost:7601/v1/sandboxes/s-demo01
curl -s -X POST localhost:7601/v1/sandboxes/s-demo01/execv2 \
  -d '{"cmd":["/bin/sh","-c","hostname"],"work_dir":"/","timeout_s":10,"max_output_bytes":100000}'
curl -sN -X POST localhost:7601/v1/sandboxes/s-demo01/execstream \
  -d '{"cmd":["tail","-f","/var/log/app.log"],"timeout_s":60}'
curl -s "localhost:7601/v1/sandboxes/s-demo01/logs?tail=100&follow=1"
curl -s -X POST localhost:7601/v1/sandboxes/s-demo01/stop -d '{"timeout_sec":5}'
curl -s -X POST localhost:7601/v1/sandboxes/s-demo01/remove
```

## RunSpec field reference

See `docs/RUNSPEC-MAPPING.md` for the field-by-field mapping and
`docs/OPS-MAPPING.md` for op-level semantics and verification status.
