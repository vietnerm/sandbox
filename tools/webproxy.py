#!/usr/bin/env python3
"""webproxy.py — host-side TCP->unix relay for minict web instances (issue #3).

Listens on 127.0.0.1:<tcp-port> (host netns) and relays every connection to
the unix socket bound INSIDE the sandbox by minict's in-sandbox relay
(<instance-rootfs>/<MINICT_PROXY_SOCK>). AF_UNIX is filesystem-scoped, not
netns-scoped, so the host reaches a socket that lives on the instance rootfs
disk even though the httpd behind it is isolated in its own netns.

Security (S1, issue #41, D31 — CWE-59 link following):
The socket file lives on the instance rootfs disk, which the payload shares
the host uid for and can rewrite. A hostile payload can unlink the relay
socket and put a SYMLINK in its place (or symlink an ancestor dir), making
the host proxy connect to an arbitrary HOST unix socket. Plain
connect(path) would follow it — every time, with a TOCTOU window.

This relay therefore resolves the socket symlink-proof, per connection:

  1. the rootfs dir is opened once with O_DIRECTORY|O_NOFOLLOW (dirfd);
  2. every in-root path component is opened openat(..., O_NOFOLLOW) —
     an ancestor symlink makes the walk fail instead of following it;
  3. the final component is opened openat(O_PATH|O_NOFOLLOW) and fstat()
     must say S_ISSOCK — anything else (symlink, file, dir, fifo) is
     refused and the connection is dropped (fail-closed);
  4. connect() goes through "/proc/self/fd/<fd>": the kernel magic link
     resolves to the inode pinned by the fd, so a swap between check and
     connect still reaches the ORIGINAL socket, never the attacker's.

Usage: webproxy.py --root <instance-rootfs-dir> --sock <in-root-socket-path>
                    --port <tcp-port>
"""
import argparse
import asyncio
import os
import stat
import sys

ARGS = None


def resolve_socket_fd(rootfd: int, sock_in_root: str) -> int:
    """Resolve <sock_in_root> under dir <rootfd> without following symlinks.

    Returns an O_PATH fd pinned to the socket inode. Raises OSError/ValueError
    on ANY symlink, non-socket file, or '..' component (fail-closed).
    """
    parts = [p for p in sock_in_root.split("/") if p not in ("", ".")]
    if not parts or ".." in parts:
        raise ValueError("bad in-root socket path")
    d = rootfd
    opened = []
    try:
        for comp in parts[:-1]:
            d = os.open(
                comp,
                os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
                dir_fd=d,
            )
            opened.append(d)
        fd = os.open(
            parts[-1],
            os.O_PATH | os.O_NOFOLLOW | os.O_CLOEXEC,
            dir_fd=d,
        )
    finally:
        for o in opened:
            os.close(o)
    st = os.fstat(fd)
    if not stat.S_ISSOCK(st.st_mode):
        os.close(fd)
        raise ValueError("proxy socket path is not a socket (symlink or tampered)")
    return fd


async def pipe(src: asyncio.StreamReader, dst_w, forward_eof: bool) -> None:
    try:
        while True:
            chunk = await src.read(65536)
            if not chunk:
                break
            dst_w.write(chunk)
            await dst_w.drain()
    except (ConnectionError, OSError):
        pass
    finally:
        if forward_eof:
            try:
                dst_w.write_eof()
            except (OSError, RuntimeError):
                pass


async def handle(c_reader: asyncio.StreamReader, c_writer: asyncio.StreamWriter):
    fd = None
    try:
        fd = resolve_socket_fd(ARGS.rootfd, ARGS.sock)
        # /proc/self/fd/<fd> magic link: kernel resolves to the inode the fd
        # pins — immune to path swaps after the open (TOCTOU-free connect).
        u_reader, u_writer = await asyncio.open_unix_connection(
            f"/proc/self/fd/{fd}"
        )
    except (OSError, ValueError):
        # fail-closed: refuse to serve a tampered/missing socket
        c_writer.close()
        return
    finally:
        if fd is not None:
            os.close(fd)
    try:
        await asyncio.gather(
            pipe(c_reader, u_writer, True),
            pipe(u_reader, c_writer, True),
        )
    finally:
        for w in (u_writer, c_writer):
            try:
                w.close()
            except OSError:
                pass


async def main() -> None:
    server = await asyncio.start_server(handle, "127.0.0.1", ARGS.port)
    async with server:
        await server.serve_forever()


if __name__ == "__main__":
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--root", required=True,
                   help="instance rootfs dir on the host (opened O_NOFOLLOW)")
    p.add_argument("--sock", required=True,
                   help="relay socket path INSIDE the root, e.g. /.minict-proxy.sock")
    p.add_argument("--port", required=True, type=int,
                   help="host TCP port, 127.0.0.1 only")
    ARGS = p.parse_args()
    if ARGS.port < 1024:
        print("webproxy: refusing privileged port", file=sys.stderr)
        sys.exit(1)
    if not ARGS.sock.startswith("/"):
        print("webproxy: --sock must be an absolute in-root path", file=sys.stderr)
        sys.exit(1)
    try:
        # O_NOFOLLOW on the root dir itself; per-connection checks do the rest.
        ARGS.rootfd = os.open(
            ARGS.root,
            os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
        )
    except OSError as e:
        print(f"webproxy: cannot open root dir safely: {e}", file=sys.stderr)
        sys.exit(2)
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        pass
