import { Context, Invokable } from 'faas-js-runtime';

/**
 * An HTTP handling function which reads the HTTP request
 * method from the context object and invokes the appropriate
 * request handler.
 * @param {Context} context a context object containing the HTTP request data
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
