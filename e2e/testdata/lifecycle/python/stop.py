import os
import urllib.request


def new():
    return Function()


class Function:
    def __init__(self):
        self._recorder_url = os.environ.get("RECORDER_URL", "")

    def stop(self):
        if self._recorder_url:
            req = urllib.request.Request(
                self._recorder_url + "/record?id=stop-python",
                method="POST",
            )
            try:
                urllib.request.urlopen(req, timeout=5)
            except Exception:
                pass

    async def handle(self, scope, receive, send):
        await send({
            "type": "http.response.start",
            "status": 200,
            "headers": [[b"content-type", b"text/plain"]],
        })
        await send({
            "type": "http.response.body",
            "body": b"OK",
        })
