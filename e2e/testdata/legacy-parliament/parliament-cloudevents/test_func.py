from cloudevents.http import CloudEvent
from parliament import Context

import unittest

func = __import__("func")

class TestFunc(unittest.TestCase):

  def test_func(self):
    # Create a CloudEvent
    # - The CloudEvent "id" is generated if omitted. "specversion" defaults to "1.0".
    attributes = {
        "type": "dev.knative.function",
        "source": "https://knative.dev/python.event",
    }
    data = {"message": "Hello World!"}
    event = CloudEvent(attributes, data)
    context = Context(req=None)
    context.cloud_event = event

    body = func.main(context)
    self.assertEqual(body.data,  event.data)

if __name__ == "__main__":
  unittest.main()
