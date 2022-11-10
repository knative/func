/**
 * Your HTTP handling function, invoked with each request. This is an example
 * function that echoes its input to the caller, and returns an error if
 * the incoming request is something other than an HTTP POST or GET.
 *
 * In can be invoked with 'func invoke'
 * It can be tested with 'npm test'
 *
 * @param {Context} context a context object.
 * @param {object} context.body the request body if any
 * @param {object} context.query the query string deserialized as an object, if any
 * @param {object} context.log logging object with methods for 'info', 'warn', 'error', etc.
 * @param {object} context.headers the HTTP request headers
 * @param {string} context.method the HTTP request method
 * @param {string} context.httpVersion the HTTP protocol version
 * See: https://github.com/knative/func/blob/main/docs/function-developers/nodejs.md#the-context-object
 */
const handle = async (context, body) => {
  // YOUR CODE HERE
  context.log.info("query", context.query);
  context.log.info("body", body);

  // If the request is an HTTP POST, the context will contain the request body
  if (context.method === 'POST') {
    return { body };
  } else if (context.method === 'GET') {
  // If the request is an HTTP GET, the context will include a query string, if it exists
  return {
      query: context.query,
    }
  } else {
    return { statusCode: 405, statusMessage: 'Method not allowed' };
  }
}

// Export the function
module.exports = { handle };
