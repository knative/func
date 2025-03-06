# Python Scaffolding

The scaffolding for Python packages consist of one directory for each of
the various invocation methods; currently "http" (default) and "cloudevents".

There are different method signatures and underlying middleware implementations
for each; hence the separation.

Note that the "instanced" versions also support static, thus only they are
included:

Each of the two support either the instanced method signature ("new") or the
static "handle" method.  This differs from strongly typed languages such as Go
which require different scaffolding be written based on this division.
This is because as a dynamically typed language, we can inspect the user's
function at runtime and import either their "handle" or "new" depending on
which was implemented. Therefore the Python scaffolding moves the complexity
into the middleware, rather than relying on the scaffolding process of func to
do static code analysis.
