from werkzeug import Request
from typing_extensions import TypedDict
from cloudevents.http import CloudEvent, to_binary

class Context(TypedDict):
    request: Request
    cloud_event: CloudEvent

def main(context: Context):
  """ 
  Function template
  The context parameter contains the Flask request object and any
  CloudEvent received with the request.
  """
  # print(f"Method: {context['request'].method}")
  attributes = {
    "type": "com.example.fn",
    "source": "https://example.com/fn"
  }
  data = { "message": "Howdy!" }
  event = CloudEvent(attributes, data)
  headers, body = to_binary(event)
  return body, 200, headers