// sandbox — CLI client for the minictd REST API (issue #91, GĐ2 mục 1).
//
// It is a thin shell over contract.HTTPClient, which already implements the
// full contract.Runtime interface over HTTP: the CLI adds no logic, no
// defaults, no state — everything it sends is exactly what the operator
// typed, mirroring the "NO MAGIC DEFAULTS" rule of the contract package.
//
// Usage:
//
//	sandbox --addr 127.0.0.1:7601 [--token ...] <command> [args]
//
// Commands:
//
//	list [prefix]           list instance names
//	up <name> <image>       run a new instance (image: base | dir:<path> |
//	                        tar:<path> | oci:<ref>); flags below
//	start <name>            start a stopped instance (wake-recreate)
//	stop <name> [--sec N]   stop (TERM, then KILL after N seconds)
//	rm <name>               remove the instance directory
//	exec <name> [cmd...]    exec a command, print stdout/stderr, exit code
//	inspect <name>          docker-shaped inspect JSON
//	stats <name>            one resource sample as JSON
//	logs <name> [--tail N]  stream instance log (no follow in v1)
//	version                 daemon version
//
// Token resolution (first hit wins): --token flag, $MINICTD_TOKEN,
// $MINICTD_TOKEN_FILE (first line), ~/.config/minictd/token (what
// minictd/deploy/install.sh writes).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vietnerm/sandbox/minictd/contract"
)

const usage = `sandbox — minictd client (issue #91). See package comment for commands.`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sandbox:", err)
		os.Exit(1)
	}
}

func run() error {
	var addr, token, tokenFile string
	args := os.Args[1:]
	// Extract global flags anywhere before the command, like a minimal
	// getopt: only --addr/--token/--token-file are recognized here.
loop:
	for len(args) > 0 {
		switch args[0] {
		case "--addr":
			if len(args) < 2 {
				return errUsage("--addr needs a value")
			}
			addr, args = args[1], args[2:]
		case "--token":
			if len(args) < 2 {
				return errUsage("--token needs a value")
			}
			token, args = args[1], args[2:]
		case "--token-file":
			if len(args) < 2 {
				return errUsage("--token-file needs a value")
			}
			tokenFile, args = args[1], args[2:]
		default:
			break loop
		}
	}
	if len(args) == 0 {
		return errUsage(usage)
	}

	if addr == "" {
		addr = os.Getenv("MINICTD_ADDR")
	}
	if addr == "" {
		addr = "127.0.0.1:7601"
	}
	base := addr
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}

	rt := contract.NewHTTPClient(base, resolveToken(token, tokenFile))
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "list":
		prefix := ""
		if len(rest) > 0 {
			prefix = rest[0]
		}
		names, err := rt.ListByNamePrefix(ctx, prefix)
		if err != nil {
			return err
		}
		if names == nil {
			names = []string{}
		}
		return emitJSON(names)

	case "up":
		if len(rest) < 2 {
			return errUsage("up <name> <image> [--cmd \"...\"] [--env K=V ...] [--webport N ...]")
		}
		spec := contract.RunSpec{Name: rest[0], Image: rest[1]}
		rest = rest[2:]
		for len(rest) > 0 {
			if len(rest) < 2 {
				return errUsage("up: flag %q needs a value", rest[0])
			}
			switch rest[0] {
			case "--cmd":
				spec.Cmd = splitList(rest[1])
			case "--env":
				spec.Env = append(spec.Env, rest[1])
			case "--webport":
				spec.Labels = append(spec.Labels, contract.LabelMinictWebports+"="+rest[1])
			case "--hostname":
				spec.Hostname = rest[1]
			case "--volume", "--v":
				spec.Volumes = append(spec.Volumes, rest[1])
			default:
				return errUsage("up: unknown flag %q (supported: --cmd --env --webport --hostname --volume)", rest[0])
			}
			rest = rest[2:]
		}
		id, err := rt.Run(ctx, spec)
		if err != nil {
			return err
		}
		fmt.Println(id)
		return nil

	case "start", "rm":
		if len(rest) != 1 {
			return errUsage("%s <name>", cmd)
		}
		if cmd == "start" {
			return rt.Start(ctx, rest[0])
		}
		return rt.Remove(ctx, rest[0])

	case "stop":
		sec := 10
		if len(rest) < 1 {
			return errUsage("stop <name> [--sec N]")
		}
		name := rest[0]
		if len(rest) >= 3 && rest[1] == "--sec" {
			if _, err := fmt.Sscanf(rest[2], "%d", &sec); err != nil {
				return errUsage("stop: bad --sec %q", rest[2])
			}
		}
		return rt.Stop(ctx, name, sec)

	case "exec":
		if len(rest) < 2 {
			return errUsage("exec <name> <cmd...>")
		}
		res, err := rt.ExecV2(ctx, rest[0], contract.ExecV2Request{Cmd: rest[1:]})
		if err != nil {
			return err
		}
		os.Stdout.WriteString(res.Stdout)
		os.Stderr.WriteString(res.Stderr)
		if res.ExitCode != 0 {
			os.Exit(res.ExitCode)
		}
		return nil

	case "inspect":
		if len(rest) != 1 {
			return errUsage("inspect <name>")
		}
		cj, err := rt.Inspect(ctx, rest[0])
		if err != nil {
			return err
		}
		return emitJSON(cj)

	case "stats":
		if len(rest) != 1 {
			return errUsage("stats <name>")
		}
		st, err := rt.Stats(ctx, rest[0])
		if err != nil {
			return err
		}
		return emitJSON(st)

	case "logs":
		if len(rest) < 1 {
			return errUsage("logs <name> [--tail N]")
		}
		tail := 0
		if len(rest) >= 3 && rest[1] == "--tail" {
			fmt.Sscanf(rest[2], "%d", &tail)
		}
		rc, err := rt.Logs(ctx, rest[0], tail, false)
		if err != nil {
			return err
		}
		defer rc.Close()
		_, err = io.Copy(os.Stdout, rc)
		return err

	case "version":
		v, err := rt.ServerVersion(ctx)
		if err != nil {
			return err
		}
		return emitJSON(map[string]string{"version": v})

	default:
		return errUsage("unknown command %q — %s", cmd, usage)
	}
}

// resolveToken implements the resolution order documented in the header.
// Never echoes the token.
func resolveToken(flagVal, flagFile string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv("MINICTD_TOKEN"); v != "" {
		return v
	}
	f := flagFile
	if f == "" {
		if env := os.Getenv("MINICTD_TOKEN_FILE"); env != "" {
			f = env
		} else if home, err := os.UserHomeDir(); err == nil {
			f = filepath.Join(home, ".config", "minictd", "token")
		}
	}
	if f == "" {
		return ""
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return ""
	}
	// SIA PR #92 (low): shared-host hygiene — a token readable by
	// group/other defeats the whole point of the bearer token. Warn, do
	// not fail: the operator may have a reason, but they must see it.
	if fi, statErr := os.Stat(f); statErr == nil && fi.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "sandbox: warning: token file %s is group/other readable (chmod 600 advised)\n", f)
	}
	return strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
}

// splitList accepts both "a b c" (spaces) and comma-separated tokens, the
// two shapes test suites and humans naturally produce.
func splitList(s string) []string {
	if s == "" {
		return nil
	}
	if strings.Contains(s, ",") {
		return strings.Split(s, ",")
	}
	return strings.Fields(s)
}

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func errUsage(f string, a ...any) error {
	return fmt.Errorf(f+"\n%s", append(a, usage)...)
}
