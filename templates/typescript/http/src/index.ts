import { Context, StructuredReturn } from 'faas-js-runtime';

/**
 * Your HTTP handling function, invoked with each request. This is an example
 * function that returns "OK" for all requests.
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
  context.log.info(`body: ${body}`);
  return {
    body: 'OK',
    headers: {
      'content-type': 'text/plain'
    }
  };
};

export { handle };
