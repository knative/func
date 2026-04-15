"""
Utility to raise the process soft open-file limit to match the hard limit.

Platforms such as some container runtimes default the soft limit to 1024,
which causes Python functions to fail under load.  Go and Java runtimes do
this automatically; this module replicates that behaviour for Python.
"""
import logging

# Safe maximum used when the hard limit is RLIM_INFINITY (no hard cap set).
_MAX_NOFILE = 65536


def configure():
    """Raise the soft open-file limit toward the hard limit, if possible."""
    try:
        import resource
    except ImportError:
        # resource module is Unix-only; skip on non-Unix platforms.
        return
    try:
        soft, hard = resource.getrlimit(resource.RLIMIT_NOFILE)
        if soft < hard:
            # RLIM_INFINITY means no hard cap is set by the kernel.
            # Passing it directly to setrlimit raises an OSError on most
            # systems, so cap the target at a known-safe value instead.
            if hard == resource.RLIM_INFINITY:
                target = _MAX_NOFILE
            else:
                target = min(hard, _MAX_NOFILE)
            resource.setrlimit(resource.RLIMIT_NOFILE, (target, hard))
            logging.info("Raised open-file limit from %d to %d", soft, target)
    except (ValueError, OSError) as e:
        logging.warning("Could not raise open-file limit: %s", e)
