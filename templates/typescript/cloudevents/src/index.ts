import { CloudEvent } from 'cloudevents';
import { Context } from 'faas-js-runtime';

/**
 * Your CloudEvents function, invoked with each request. This
 * is an example function which logs the incoming event and returns
 * a CloudEvent with "OK" as the data.
 *
 * It can be invoked with 'func invoke'.
 * It can be tested with 'npm test'.
 *
 * @param {Context} context a context object.
 * @param {object} context.body the request body if any
 * @param {object} context.query the query string deserialzed as an object, if any
 * @param {object} context.log logging object with methods for 'info', 'warn', 'error', etc.
 * @param {object} context.headers the HTTP request headers
 * @param {string} context.method the HTTP request method
 * @param {string} context.httpVersion the HTTP protocol version
 * See: https://github.com/knative/func/blob/main/docs/guides/nodejs.md#the-context-object
 * @param {CloudEvent} cloudevent the CloudEvent
 */
// eslint-disable-next-line prettier/prettier
const handle = async (context: Context, cloudevent?: CloudEvent<unknown>): Promise<CloudEvent<{ message: string }>> => {
  // YOUR CODE HERE
  if (cloudevent) {
    context.log.info(`
-----------------------------------------------------------
CloudEvent:
${cloudevent}

Data:
${JSON.stringify(cloudevent.data)}
-----------------------------------------------------------
`);
  }

  return new CloudEvent<{ message: string }>({
    source: 'function',
    type: 'function.response',
    data: { message: 'OK' }
  });
};

export { handle };
