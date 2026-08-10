import threading
import time


def new():
    return Function()


class Function:
    def __init__(self):
        self.start_called = False
        self.config_value = ""
        self._ready = False
        self._alive = True
        self._nonce = str(time.time_ns())
        threading.Timer(15.0, self._become_ready).start()

    def _become_ready(self):
        self._ready = True

    def start(self, cfg):
        self.start_called = True
        self.config_value = cfg.get("TEST_CONFIG_VALUE", "")

    def ready(self):
        return self._ready, "ready" if self._ready else "warming up"

    def alive(self):
        return self._alive, "alive" if self._alive else "unhealthy"

    async def handle(self, scope, receive, send):
        path = scope.get("path", "/")

        if path == "/set-unhealthy":
            self._alive = False
            body = b"UNHEALTHY"
        elif self.start_called:
            body = f"START_OK:{self.config_value} READY={str(self._ready).lower()} ALIVE={str(self._alive).lower()} NONCE={self._nonce}".encode()
        else:
            body = f"START_NOT_CALLED READY={str(self._ready).lower()} ALIVE={str(self._alive).lower()} NONCE={self._nonce}".encode()

        await send({
            "type": "http.response.start",
            "status": 200,
            "headers": [[b"content-type", b"text/plain"]],
        })
        await send({
            "type": "http.response.body",
            "body": body,
        })
