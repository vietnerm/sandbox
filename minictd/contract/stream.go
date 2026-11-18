package contract

// StreamFrame is the NDJSON wire format of POST /v1/sandboxes/{name}/execstream
// (and the wire format the HTTP client re-materializes into pipes):
//
//	{"t":"stdout","d":"<bytes>"}    — payload chunk (base64 in JSON)
//	{"t":"stderr","d":"<bytes>"}
//	{"t":"exit","exit_code":N,"timed_out":bool,"duration_ms":N}
//
// stdout/stderr frames interleave as the daemon reads them; exactly one
// exit frame terminates the stream. Go's encoding/json encodes []byte as
// std-base64 automatically, so Data needs no manual encoding.
type StreamFrame struct {
	T          string `json:"t"`
	Data       []byte `json:"d,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}
