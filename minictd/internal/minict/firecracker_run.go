// Package minict — firecracker_run.go (GĐ4, issue #193).
//
// runFirecracker is the GĐ4 backend entry point. When /dev/kvm is available,
// it spawns a Firecracker microVM via the jailer. Without /dev/kvm, it
// returns ErrNoKVM (env-gated, same pattern as overlay D66 / pasta D67).

package minict

import (
	"context"
	"fmt"
	"time"

	"github.com/vietnerm/sandbox/minictd/contract"
	"github.com/vietnerm/sandbox/minictd/internal/firecracker"
)

// runFirecracker handles the "firecracker" backend Run path (GĐ4).
// Without /dev/kvm, returns a clear env-gated error.
func (s *Store) runFirecracker(ctx context.Context, spec contract.RunSpec, start time.Time) (string, error) {
	b := firecracker.NewFirecrackerBackend(s.Root, "", "", "")
	vcpu := 1
	if spec.CPUs != "" {
		// parse CPUs string (e.g. "0.5", "2")
		var f float64
		_, _ = fmt.Sscanf(spec.CPUs, "%f", &f)
		if f > 0 {
			vcpu = int(f)
			if vcpu < 1 {
				vcpu = 1
			}
		}
	}
	memMib := 256
	if spec.Memory != "" {
		if bytes, err := parseDockerSize(spec.Memory); err == nil && bytes > 0 {
			memMib = int(bytes / (1024 * 1024))
			if memMib < 64 {
				memMib = 64
			}
		}
	}
	name, err := b.Run(ctx, spec.Name, vcpu, memMib, "", "")
	if err != nil {
		s.eventLogErr("create", spec.Name, start, err)
		return "", err
	}
	s.eventLog("create", name, "ok", start, "backend", "firecracker")
	return name, nil
}
