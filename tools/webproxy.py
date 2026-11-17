#!/usr/bin/env python3
"""webproxy.py — host-side TCP->unix relay for minict web instances (issue #3).

Listens on 127.0.0.1:<tcp-port> (host netns) and relays every connection to
the unix socket bound INSIDE the sandbox by minict's in-sandbox relay
(<instance-rootfs>/<MINICT_PROXY_SOCK>). AF_UNIX is filesystem-scoped, not
netns-scoped, so the host reaches a socket that lives on the instance rootfs
disk even though the httpd behind it is isolated in its own netns.

Usage: webproxy.py --unix <path> --port <tcp-port>
"""
import argparse
import asyncio
import sys

ARGS = None


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
    try:
        u_reader, u_writer = await asyncio.open_unix_connection(ARGS.unix)
    except (OSError, ConnectionError):
        c_writer.close()
        return
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
    p.add_argument("--unix", required=True, help="unix socket path (instance rootfs)")
    p.add_argument("--port", required=True, type=int, help="host TCP port, 127.0.0.1 only")
    ARGS = p.parse_args()
    if ARGS.port < 1024:
        print("webproxy: refusing privileged port", file=sys.stderr)
        sys.exit(1)
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        pass
