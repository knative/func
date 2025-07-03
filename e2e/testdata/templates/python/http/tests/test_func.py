"""
An example set of unit tests which confirm that the main handler (the
callable function) returns 200 OK for a simple HTTP GET.
"""
import pytest
from function import new


@pytest.mark.asyncio
async def test_function_handle():
    f = new()  # Instantiate Function to Test

    sent_ok = False
    sent_headers = False
    sent_body = False

    # Mock Send
    async def send(message):
        nonlocal sent_ok
        nonlocal sent_headers
        nonlocal sent_body

        if message.get('status') == 200:
            sent_ok = True

        if message.get('type') == 'http.response.start':
            sent_headers = True

        if message.get('type') == 'http.response.body':
            sent_body = True

    # Invoke the Function
    await f.handle({}, {}, send)

    # Assert send was called
    assert sent_ok, "Function did not send a 200 OK"
    assert sent_headers, "Function did not send headers"
    assert sent_body, "Function did not send a body"
