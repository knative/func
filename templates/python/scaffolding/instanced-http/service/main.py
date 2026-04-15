"""
This code is glue between a user's Function and the middleware which will
expose it as a network service.  This code is written on-demand when a
Function is being built, deployed or run.  This will be included in the
final container.
"""
import logging
import resource
from func_python.http import serve

logging.basicConfig(level=logging.INFO)

# Raise the soft limit for open files to match the hard limit.
# Platforms such as some container runtimes default the soft limit to 1024,
# which causes failures under load.  Go and Java runtimes do this
# automatically; we replicate the behaviour here.
try:
    _soft, _hard = resource.getrlimit(resource.RLIMIT_NOFILE)
    if _soft < _hard:
        resource.setrlimit(resource.RLIMIT_NOFILE, (_hard, _hard))
        logging.info("Raised open-file limit from %d to %d", _soft, _hard)
except Exception as e:
    logging.warning("Could not raise open-file limit: %s", e)

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
