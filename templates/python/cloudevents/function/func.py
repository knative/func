# Function
import logging
from cloudevents.http import CloudEvent


def new():
    """ New is the only method that must be implemented by a Function.
    The instance returned can be of any name.
    """
    return Function()


class Function:
    def __init__(self):
        """ The init method is an optional method where initialization can be
        performed. See the start method for a startup hook which includes
        configuration.
        """

    async def handle(self, scope, receive, send):
        """ Handle all HTTP requests to this Function other than readiness
        and liveness probes.  The CloudEvent from the request is already
        decoded and available in the scope as 'event'.

        The 'send' method can be used to directly send a structured CloudEvent
        as a response.

        To send a binary encoded CloudEvent use `send.binary(myResponseEvent)`.
        """
        logging.info("Request Received")

        # 1) Extract the CloudEvent from the scope
        request = scope["event"]

        # 2) Create a new CloudEvent as response
        response = CloudEvent({
            "type": "dev.knative.function.response",
            "source": "https://knative.dev/python-function-response",
            "id": f"response-{request.get('id', 'unknown')}"
        })

        # 3) Set the response's data field to the request event's data field
        response.data = request.data

        # 4) Send the response CloudEvent
        # The 'send' method is already decorated with CloudEvent middleware
        await send(response)

    def start(self, cfg):
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

    def alive(self):
        """ alive is an optional method for performing a deep check on your
        Function's liveness.  If removed, the system will assume the function
        is ready if the process is running. This is exposed by default at the
        path /health/liveness.  The optional string return is a message.
        """
        return True, "Alive"

    def ready(self):
        """ ready is an optional method for performing a deep check on your
        Function's readiness.  If removed, the system will assume the function
        is ready if the process is running.  This is exposed by default at the
        path /health/rediness.
        """
        return True, "Ready"
