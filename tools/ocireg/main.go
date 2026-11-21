package main

// ocireg — in-memory OCI registry harness for the minictd `oci:` e2e cases
// (issue #103, D51). NOT production code: it exists so the suite can push
// images with the REAL crane client and pull them through the REAL
// `unpackOCI` path (crane export -> secureUnpack) fully offline — no
// docker.io (anonymous rate-limit flake class on shared runner IPs), no
// ttl.sh (third-party uptime).
//
// Bind 127.0.0.1 ONLY. Plain HTTP is fine for a loopback harness:
// go-containerregistry's remote falls back from https to http when the
// TCP connect is refused on the tls attempt (measured, journal/2026-11-21/
// research.md Phiên 16), so crane needs no --insecure against this server.
//
// Kept in a SEPARATE module so minictd/go.mod stays free of
// go-containerregistry (the daemon shells out to the crane binary — D50
// architecture unchanged).

import (
	"log"
	"net/http"
	"os"

	"github.com/google/go-containerregistry/pkg/registry"
)

func main() {
	addr := "127.0.0.1:15100"
	if len(os.Args) > 1 {
		addr = "127.0.0.1:" + os.Args[1]
	}
	log.Printf("ocireg (test harness) listening %s", addr)
	log.Fatal(http.ListenAndServe(addr, registry.New()))
}
