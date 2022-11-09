import { CloudEvent } from 'cloudevents';
import { Context } from 'faas-js-runtime';

/**
 * Your CloudEvents function, invoked with each request. This
 * is an example function which logs the incoming event and echoes
 * the received event data to the caller.
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
const handle = async (context: Context, cloudevent?: CloudEvent<Customer>): Promise<CloudEvent<Customer|string>> => {
  // YOUR CODE HERE
  const meta = {
    source: 'function.eventViewer',
    type: 'echo'
  };
  // The incoming CloudEvent
  if (!cloudevent) {
    const response: CloudEvent<string> = new CloudEvent<string>({
      ...meta,
      ...{ type: 'error', data: 'No event received' }
    });
    context.log.info(response.toString());
    return response;
  }
  context.log.info(`
-----------------------------------------------------------
CloudEvent:
${cloudevent}

Data:
${JSON.stringify(cloudevent.data)}
-----------------------------------------------------------
`);
  // respond with a new CloudEvent
  return new CloudEvent<Customer>({ ...meta, data: cloudevent.data });
};

export interface Customer {
  name: string;
  customerId: string;
}

export { handle };
