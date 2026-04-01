import logging
from .mylib import greet


def new():
    return Function()


class Function:
    async def handle(self, scope, receive, send):
        logging.info("Request Received")
        await send({
            'type': 'http.response.start',
            'status': 200,
            'headers': [[b'content-type', b'text/plain']],
        })
        await send({
            'type': 'http.response.body',
            'body': greet().encode(),
        })
