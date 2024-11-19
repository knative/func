# Function
import logging
import typing
import urllib.parse


def new():
    """ New is the only method that must be implemented by a Function.
    The instance returned can be of any name, but it must be "callable".
    See the example below.
    """
    logging.info("returning a new function")
    return Function()


class Function:
    """ Function is the structure representing the deployed function as a
    network service.  The class name can be changed.  The only required method
    is "__call__" which makes it a callable ASGI request handler.
    """

    def __init__(self):
        """ The init method is an optional method where initialization can be
        performed. See the start method for a startup hook which includes
        configuration.
        """
        logging.info("Function instantiating")

    async def __call__(self, scope, receive, send):
        """ __call__ is the required method which handles HTTP requests and is
        exposed at the root path "https://myfunc.example.com/".
        The default implementation here echoes the HTTP request received.
        """

        # Build a string representation of the inbound HTTP request
        request = f"{scope['method']} {scope['path']}"
        query = scope['query_string'].decode('utf-8')
        if query:
            request += f"?{urllib.parse.unquote(query)}"
        headers = "\r\n".join([f"{k.decode('utf-8')}: {v.decode('utf-8')}"
                              for k, v in scope['headers']])
        request = request + "HTTP/1.1\r\n" + headers

        # Log the request locally
        logging.info("OK: " + request)

        # echo the request to the calling client
        await send({
            'type': 'http.response.start',
            'status': 200,
            'headers': [
                [b'content-type', b'text/plain'],
            ],
        })
        await send({
            'type': 'http.response.body',
            'body': request.encode('utf-8'),
        })

    def start(self, cfg: typing.Dict[str, str]):
        """ start is an optional method which is called when a new Function
        instance is started, such as when scaling up or during an update.
        Provided is a dictionary containing all environmental configuration.
        Args:
            cfg (Dict[str, str]): A dictionary containing environmental config.
                In most cases this will be a copy of os.environ, but it is
                best practice to use this cfg dict instead of os.environ.
        """
        logging.info("Function starting")

    def stop(self):
        """ stop is an optional method which is called when a function is
        stopped, such as when scaled down, updated, or manually canceled.  Stop
        can block while performing function shutdown/cleanup operations.  The
        process will eventually be killed if this method blocks beyond the
        platform's configured maximum studown timeout.
        """
        logging.info("Function stopping")

    def alive(self) -> tuple[bool, str | None]:
        """ alive is an optional method for performing a deep check on your
        Function's liveness.  If removed, the system will assume the function
        is ready if the process is running. This is exposed by default at the
        path /health/liveness.  The optional string return is a message.
        """
        return True, "Alive"

    def ready(self) -> tuple[bool, str | None]:
        """ ready is an optional method for performing a deep check on your
        Function's readiness.  If removed, the system will assume the function
        is ready if the process is running.  This is exposed by default at the
        path /health/rediness.
        """
        return True, "Ready"
