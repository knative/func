"""
An example set of unit tests which confirm that the main handler (the
callable function) reuturns true for a simple HTTP GET, and that both
the readiness and liveness endpoints return true.   Modify as needed to
reflect the features of your Function instance.

Functions are currently served as ASGI applications using hypercorn, so we
use httpx for testing.  To run the tests using poetry:

poetry run python -m unittest discover
"""
import unittest
from httpx import AsyncClient, ASGITransport
from function import new


class TestFunc(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        self.f = new()  # Instantiate the function

    async def test_call(self):
        transport = ASGITransport(app=self.f)
        # Use AsyncClient to simulate an TTP request to the ASGI app
        async with AsyncClient(transport=transport, base_url="http://server") as client:
            response = await client.get("/")
            self.assertEqual(response.status_code, 200)

    async def test_alive(self):
        self.assertTrue(self.f.alive(), "'alive' method did not return True")

    async def test_ready(self):
        self.assertTrue(self.f.ready(), "'ready' method did not return True")


if __name__ == '__main__':
    unittest.main()
