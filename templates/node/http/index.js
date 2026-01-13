/**
 * Your HTTP handling function, invoked with each request. This is an example
 * function that echoes its input to the caller, and returns an error if
 * the incoming request is something other than an HTTP POST or GET.
 *
 * It can be invoked with 'func invoke'
 * It can be tested with 'npm test'
 *
 * @param {Context} context - A context object.
 * @param {object} context.query - The query string deserialized as an object, if any.
 * @param {object} context.log - Logging object with methods for 'info', 'warn', 'error', etc.
 * @param {object} context.headers - The HTTP request headers.
 * @param {string} context.method - The HTTP request method.
 * @param {string} context.httpVersion - The HTTP protocol version.
 * @param {object} body - The request body if any.
 * @returns {object} HTTP response object with body, query, or error status.
 *
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
