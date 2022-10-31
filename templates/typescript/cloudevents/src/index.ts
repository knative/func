import { CloudEvent, HTTP, Message } from 'cloudevents';
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
const handle = async (_: Context, cloudevent?: CloudEvent<unknown>): Promise<Message> => {
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
    console.log(response.toString());
    return HTTP.binary(response);
  }
  console.log(`
-----------------------------------------------------------
CloudEvent:
${cloudevent}

Data:
${JSON.stringify(cloudevent.data)}
-----------------------------------------------------------
`);
  // respond with a new CloudEvent
  return HTTP.binary(new CloudEvent({ ...meta, data: cloudevent.data }));
};

export { handle };
