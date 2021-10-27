import { Context, Invokable } from 'faas-js-runtime';

/**
 * Your HTTP handling function, invoked with each request. This is an example
 * function that logs the incoming request and echoes its input to the caller.
 *
 * @param {Context} context a context object.
 * @param {object} context.body the request body if any
 * @param {object} context.query the query string deserialzed as an object, if any
 * @param {object} context.log logging object with methods for 'info', 'warn', 'error', etc.
 * @param {object} context.headers the HTTP request headers
 * @param {string} context.method the HTTP request method
 * @param {string} context.httpVersion the HTTP protocol version
 * See: https://github.com/knative-sandbox/kn-plugin-func/blob/main/docs/guides/nodejs.md#the-context-object
 */
export const handle: Invokable = function (context: Context): string {
  // eslint-disable-next-line no-console
  console.log(`
-----------------------------------------------------------
Headers:
${JSON.stringify(context.headers)}

Query:
${JSON.stringify(context.query)}

Body:
${JSON.stringify(context.body)}
-----------------------------------------------------------
`);
  return JSON.stringify(context.body);
};
