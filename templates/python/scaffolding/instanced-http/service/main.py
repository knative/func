"""
This code is glue between a user's Function and the middleware which will
expose it as a network service.  This code is written on-demand when a
Function is being built, deployed or run.  This will be included in the
final container.
"""
import logging
from func_python.http import serve

logging.basicConfig(level=logging.INFO)

try:
    from function import new as handler  # type: ignore[import]
except ImportError:
    try:
        from function import handle as handler  # type: ignore[import]
    except ImportError:
        logging.error("Function must export either 'new' or 'handle'")
        raise

if __name__ == "__main__":
    logging.info("Functions middleware invoking user function")
    serve(handler)
