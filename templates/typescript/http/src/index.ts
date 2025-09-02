import { Context, StructuredReturn } from 'faas-js-runtime';

/**
 * Your HTTP handling function, invoked with each request. This is an example
 * function that logs the incoming request and echoes its input to the caller.
 *
 * It can be invoked with `func invoke`
 * It can be tested with `npm test`
 *
 * It can be invoked with `func invoke`
 * It can be tested with `npm test`
 *
 * @param {Context} context a context object.
 * @param {object} context.body the request body if any
 * @param {object} context.query the query string deserialized as an object, if any
 * @param {object} context.log logging object with methods for 'info', 'warn', 'error', etc.
 * @param {object} context.headers the HTTP request headers
 * @param {string} context.method the HTTP request method
 * @param {string} context.httpVersion the HTTP protocol version
 * See: https://github.com/knative/func/blob/main/docs/guides/nodejs.md#the-context-object
 */
const handle = async (context: Context, body: string): Promise<StructuredReturn> => {
  // YOUR CODE HERE
  context.log.info(`
-----------------------------------------------------------
Headers:
${JSON.stringify(context.headers)}

Query:
${JSON.stringify(context.query)}

Body:
${JSON.stringify(body)}
-----------------------------------------------------------
`);
  // If the request is an HTTP POST, return the request body
  if (context.method === 'POST') {
    return {
      body: body,
      headers: {
        'content-type': 'application/json'
      }
    };
  } else if (context.method === 'GET') {
    // If the request is an HTTP GET, return the query parameters
    return {
      body: context.query,
      headers: {
        'content-type': 'application/json'
      }
    };
  } else {
    return {
      statusCode: 405,
      body: { error: 'Method not allowed' },
      headers: {
        'content-type': 'application/json'
      }
    };
  }
};

export { handle };
