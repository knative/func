from parliament import Context
from flask import Request


def pretty_print(req: Request) -> str:
    ret = str(req.method) + ' ' + str(req.url) + ' ' + str(req.host) + '\n'
    for (header, values) in req.headers:
        ret += str(header) + ": " + values + '\n'

    return ret

 
def main(context: Context):
    """ 
    Function template
    The context parameter contains the Flask request object and any
    CloudEvent received with the request.
    """

    # Add your business logic here
    print("Received request")

    if 'request' in context.keys():
        ret = pretty_print(context.request)
        print(ret)
        return { "message": ret }, 200
    else:
        print("Empty request")
        return { "message": "Empty request" }, 200
