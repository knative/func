from werkzeug import Request
from typing_extensions import TypedDict

class Context(TypedDict):
    request: Request

def main(context):
  """ 
  Function template
  The context parameter contains the Flask request object and any
  CloudEvent received with the request.
  """
  return { "message": "Howdy!" }, 200
