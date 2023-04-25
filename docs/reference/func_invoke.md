## func invoke

Invoke a local or remote function

### Synopsis


NAME
	func invoke - test a function by invoking it with test data

SYNOPSIS
	func invoke [-t|--target] [-f|--format]
	             [--id] [--source] [--type] [--data] [--file] [--content-type]
	             [-s|--save] [-p|--path] [-i|--insecure] [-c|--confirm] [-v|--verbose]

DESCRIPTION
	Invokes the function by sending a test request to the currently running
	function instance, either locally or remote.  If the function is running
	both locally and remote, the local instance will be invoked.  This behavior
	can be manually overridden using the --target flag.

	Functions are invoked with a test data structure consisting of five values:
		id:            A unique identifier for the request.
		source:        A sender name for the request (sender).
		type:          A type for the request.
		data:          Data (content) for this request.
		content-type:  The MIME type of the value contained in 'data'.

	The values of these parameters can be individually altered from their defaults
	using their associated flags. Data can also be provided from a file using the
	--file flag.

	Invocation Target
	  The function instance to invoke can be specified using the --target flag
	  which accepts the values "local", "remote", or <URL>.  By default the
	  local function instance is chosen if running (see func run).
	  To explicitly target the remote (deployed) function:
	    func invoke --target=remote
	  To target an arbitrary endpoint, provide a URL:
	    func invoke --target=https://myfunction.example.com

	Invocation Data
	  Providing a filename in the --file flag will base64 encode its contents
	  as the "data" parameter sent to the function.  The value of --content-type
	  should be set to the type from the source file.  For example, the following
	  would send a JPEG base64 encoded in the "data" POST parameter:
	    func invoke --file=example.jpeg --content-type=image/jpeg

	Message Format
	  By default functions are sent messages which match the invocation format
	  of the template they were created using; for example "http" or "cloudevent".
	  To override this behavior, use the --format (-f) flag.
	    func invoke -f=cloudevent -t=http://my-sink.my-cluster

EXAMPLES

	o Invoke the default (local or remote) running function with default values
	  $ func invoke

	o Run the function locally and then invoke it with a test request:
	  (run in two terminals or by running the first in the background)
	  $ func run
	  $ func invoke

	o Deploy and then invoke the remote function:
	  $ func deploy
	  $ func invoke

	o Invoke a remote (deployed) function when it is already running locally:
	  (overrides the default behavior of preferring locally running instances)
	  $ func invoke --target=remote

	o Specify the data to send to the function as a flag
	  $ func invoke --data="Hello World!"

	o Send a JPEG to the function
	  $ func invoke --file=example.jpeg --content-type=image/jpeg

	o Invoke an arbitrary endpoint (HTTP POST)
		$ func invoke --target="https://my-http-handler.example.com"

	o Invoke an arbitrary endpoint (CloudEvent)
		$ func invoke -f=cloudevent -t="https://my-event-broker.example.com"

	o Allow insecure server connections when using SSL
		$ func invoke --insecure



```
func invoke
```

### Options

```
  -c, --confirm               Prompt to confirm options interactively ($FUNC_CONFIRM)
      --content-type string   Content Type of the data. ($FUNC_CONTENT_TYPE) (default "application/json")
      --data string           Data to send in the request. ($FUNC_DATA) (default "{\"message\":\"Hello World\"}")
      --file string           Path to a file to use as data. Overrides --data flag and should be sent with a correct --content-type. ($FUNC_FILE)
  -f, --format string         Format of message to send, 'http' or 'cloudevent'.  Default is to choose automatically. ($FUNC_FORMAT)
  -h, --help                  help for invoke
      --id string             ID for the request data. ($FUNC_ID)
  -i, --insecure              Allow insecure server connections when using SSL. ($FUNC_INSECURE)
  -p, --path string           Path to the function.  Default is current directory ($FUNC_PATH)
      --source string         Source value for the request data. ($FUNC_SOURCE) (default "/boson/fn")
  -t, --target string         Function instance to invoke.  Can be 'local', 'remote' or a URL.  Defaults to auto-discovery if not provided. ($FUNC_TARGET)
      --type string           Type value for the request data. ($FUNC_TYPE) (default "boson.fn")
  -v, --verbose               Print verbose logs ($FUNC_VERBOSE)
```

### SEE ALSO

* [func](func.md)	 - func manages Knative Functions

