"""
This code is glue between a user's Function and the middleware which will
expose it as a network service.  This code is written on-demand when a
Function is being built, deployed or run.  This will be included in the
final container.

To quickly test this manually (such as during development), one can use
the following process:
- Initialize Project
  `poetry init`
- Interactively added `func-python` dependency
- Initialize virtual env
  `poetry shell`
  Note that the above shell command can interfere with keyboard shortucts.
  If so, exit `deactivate && exit` (not sure why both are needed), and
  instead source only the envs necessary:
  `source $(poetry env info --path)/bin/activate`
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
