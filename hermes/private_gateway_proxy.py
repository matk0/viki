import asyncio
import os
from contextlib import suppress
from functools import partial


FORWARDS = (
    (9219, "127.0.0.1", 9119),
    (9220, "127.0.0.1", 9120),
)


def rewrite_host_header(request: bytes, target_host: str, target_port: int) -> bytes:
    header = request.removesuffix(b"\r\n\r\n")
    lines = header.split(b"\r\n")
    replacement = f"Host: {target_host}:{target_port}".encode()
    for index, line in enumerate(lines):
        if line.partition(b":")[0].lower() == b"host":
            lines[index] = replacement
            break
    else:
        lines.append(replacement)
    return b"\r\n".join(lines) + b"\r\n\r\n"


async def _relay(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
    try:
        while chunk := await reader.read(64 * 1024):
            writer.write(chunk)
            await writer.drain()
    except (ConnectionError, asyncio.CancelledError):
        pass


async def proxy_connection(
    client_reader: asyncio.StreamReader,
    client_writer: asyncio.StreamWriter,
    target_host: str,
    target_port: int,
) -> None:
    try:
        upstream_reader, upstream_writer = await asyncio.open_connection(
            target_host, target_port
        )
    except OSError:
        client_writer.close()
        with suppress(ConnectionError):
            await client_writer.wait_closed()
        return

    try:
        request = await asyncio.wait_for(
            client_reader.readuntil(b"\r\n\r\n"), timeout=10
        )
    except (asyncio.IncompleteReadError, asyncio.LimitOverrunError, TimeoutError):
        upstream_writer.close()
        client_writer.close()
        with suppress(ConnectionError):
            await upstream_writer.wait_closed()
        with suppress(ConnectionError):
            await client_writer.wait_closed()
        return

    upstream_writer.write(rewrite_host_header(request, target_host, target_port))
    await upstream_writer.drain()

    tasks = {
        asyncio.create_task(_relay(client_reader, upstream_writer)),
        asyncio.create_task(_relay(upstream_reader, client_writer)),
    }
    _, pending = await asyncio.wait(tasks, return_when=asyncio.FIRST_COMPLETED)
    for task in pending:
        task.cancel()
    await asyncio.gather(*tasks, return_exceptions=True)

    upstream_writer.close()
    client_writer.close()
    with suppress(ConnectionError):
        await upstream_writer.wait_closed()
    with suppress(ConnectionError):
        await client_writer.wait_closed()


async def create_proxies(
    bind_host: str, forwards: tuple[tuple[int, str, int], ...] = FORWARDS
) -> list[asyncio.Server]:
    servers = []
    for bind_port, target_host, target_port in forwards:
        handler = partial(
            proxy_connection,
            target_host=target_host,
            target_port=target_port,
        )
        servers.append(await asyncio.start_server(handler, bind_host, bind_port))
    return servers


async def main() -> None:
    bind_host = os.environ.get("HERMES_GATEWAY_PROXY_HOST", "0.0.0.0")
    servers = await create_proxies(bind_host)
    async with servers[0], servers[1]:
        await asyncio.gather(*(server.serve_forever() for server in servers))


if __name__ == "__main__":
    asyncio.run(main())
