# Test handler fixture for TestInt_KafkaPythonDelivery.
#
# A plain CloudEvents-over-HTTP Python function (no Kafka code) that echoes the
# received payload to its log, so the integration test can confirm the exact
# record produced to Kafka was delivered to this function -- decoded intact --
# via the injected kafka-runtime sidecar. It returns an OK response CloudEvent
# (a 2xx) so the sidecar commits the offset and the consumer group drains.
#
# This is deliberately a copy-over-the-scaffold fixture rather than the shipped
# template: the stock template only logs "Request Received" with no payload, so
# verifying specific content requires a handler that echoes it.
import logging
from cloudevents.core.v1.event import CloudEvent


def new():
    return Function()


class Function:
    async def handle(self, scope, receive, send):
        event = scope["event"]
        # The test produces a record whose value carries a unique token and
        # asserts this line (with that token) appears in the function's logs.
        logging.info(f"FUNC-E2E-RECEIVED data={event.get_data()!r}")

        response = CloudEvent(
            attributes={
                "type": "function.response",
                "source": "function",
                "id": f"response-{event.get_id() or 'unknown'}",
            },
            data={"message": "OK"},
        )
        await send(response)
