"""
Utility to raise the process soft open-file limit to match the hard limit.

Platforms such as some container runtimes default the soft limit to 1024,
which causes Python functions to fail under load.  Go and Java runtimes do
this automatically; this module replicates that behaviour for Python.
"""
import logging


def configure():
    """Raise the soft open-file limit to the hard limit, if possible."""
    try:
        import resource
    except ImportError:
        # resource module is Unix-only; skip on non-Unix platforms.
        return
    try:
        soft, hard = resource.getrlimit(resource.RLIMIT_NOFILE)
        if soft < hard:
            resource.setrlimit(resource.RLIMIT_NOFILE, (hard, hard))
            logging.info("Raised open-file limit from %d to %d", soft, hard)
    except (ValueError, OSError) as e:
        logging.warning("Could not raise open-file limit: %s", e)
