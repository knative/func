from parliament import Context

 
def main(context: Context):
    """ 
    Function template
    The context parameter contains the Flask request object and any
    CloudEvent received with the request.
    """
    return { "message": "Howdy!" }, 200
