# Python

Python support consists of two templates, "http" and "cloudevents", and
scaffolding which is used to connect user Functions to the func-python
middleware.

When a user creates a new Python Function, either the "http" (default) or
"cloudevents" template is written out as their new Function's initial state.

When a Function is built, such as on deploy, the contents of the scaffolding
directory herein is written on-demand, effectively wrapping the user's
Function in an app which uses the functions middleware to expose the Function
as a service.  This "scaffolded" Function is thus able to be containerized and
deployed.  see knative-extensions/func-python for the middleware,  which
includes examples.

The core templates are intentionally minimal.  Additional templates can be used
from templates repositories, such as the officially supported func-templates.

See the README.md in each directory for more information.
