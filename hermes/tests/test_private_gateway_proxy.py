import asyncio
import unittest

from hermes.private_gateway_proxy import create_proxies


class PrivateGatewayProxyTest(unittest.IsolatedAsyncioTestCase):
    async def test_rewrites_the_gateway_host_then_forwards_traffic(self):
        captured = []

        async def gateway(reader, writer):
            captured.append(await reader.readuntil(b"\r\n\r\n"))
            writer.write(b"HTTP/1.1 101 Switching Protocols\r\n\r\n")
            await writer.drain()
            writer.write(await reader.read(1024))
            await writer.drain()
            writer.close()
            await writer.wait_closed()

        upstream = await asyncio.start_server(gateway, "127.0.0.1", 0)
        upstream_port = upstream.sockets[0].getsockname()[1]
        proxies = await create_proxies(
            "127.0.0.1", ((0, "127.0.0.1", upstream_port),)
        )
        proxy_port = proxies[0].sockets[0].getsockname()[1]

        try:
            reader, writer = await asyncio.open_connection("127.0.0.1", proxy_port)
            writer.write(
                b"GET /api/ws HTTP/1.1\r\n"
                b"Host: hermes:9219\r\n"
                b"Upgrade: websocket\r\n\r\n"
            )
            await writer.drain()
            self.assertEqual(
                await reader.readuntil(b"\r\n\r\n"),
                b"HTTP/1.1 101 Switching Protocols\r\n\r\n",
            )
            writer.write(b"websocket-frame")
            await writer.drain()
            self.assertEqual(await reader.read(), b"websocket-frame")
            self.assertIn(
                f"Host: 127.0.0.1:{upstream_port}\r\n".encode(), captured[0]
            )
            self.assertNotIn(b"Host: hermes:9219", captured[0])
            writer.close()
            await writer.wait_closed()
        finally:
            proxies[0].close()
            upstream.close()
            await proxies[0].wait_closed()
            await upstream.wait_closed()


if __name__ == "__main__":
    unittest.main()
