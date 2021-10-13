from parliament import Context, event


@event
def main(context: Context):
    """
    Function template
    The context parameter contains the Flask request object and any
    CloudEvent received with the request.
    """
    # print(f"Method: {context.request.method}")

    # The return value here will be applied as the data attribute
    # of a CloudEvent returned to the function invoker
    return { "message": "Howdy!" }
