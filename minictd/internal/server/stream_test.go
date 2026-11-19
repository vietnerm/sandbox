package server

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/vietnerm/sandbox/minictd/contract"
)

// TestMuxStreamsKeepsLastChunk is the regression for issue #50 (MD13
// flake): with tiny, instantly-EOF streams both done signals race the
// buffered chunks, and the old loop dropped the chunks when done won.
func TestMuxStreamsKeepsLastChunk(t *testing.T) {
	for i := 0; i < 5000; i++ {
		var out, errb bytes.Buffer
		emit := func(f contract.StreamFrame) error {
			switch f.T {
			case "stdout":
				out.Write(f.Data)
			case "stderr":
				errb.Write(f.Data)
			}
			return nil
		}
		ok := muxStreams(context.Background(),
			strings.NewReader("out1\n"), strings.NewReader("err1\n"), emit)
		if !ok || out.String() != "out1\n" || errb.String() != "err1\n" {
			t.Fatalf("iter %d: ok=%v stdout=%q stderr=%q", i, ok, out.String(), errb.String())
		}
	}
}

// TestMuxStreamsCtxCancel: a vanished client returns false, never blocks.
func TestMuxStreamsCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pr, pw := newBlockingPipe()
	defer pw.Close()
	if muxStreams(ctx, pr, strings.NewReader(""), func(contract.StreamFrame) error { return nil }) {
		t.Fatal("want false on cancelled ctx")
	}
}

func newBlockingPipe() (*pipeReader, *pipeWriter) {
	ch := make(chan struct{})
	return &pipeReader{ch}, &pipeWriter{ch}
}

type pipeReader struct{ ch chan struct{} }
type pipeWriter struct{ ch chan struct{} }

func (r *pipeReader) Read([]byte) (int, error) { <-r.ch; return 0, context.Canceled }
func (w *pipeWriter) Close() error             { close(w.ch); return nil }
