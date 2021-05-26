'use strict';
import { CloudEvent, HTTP, Message } from 'cloudevents';
import { Context, Invokable } from 'faas-js-runtime';

/**
 * An example function that responds to incoming CloudEvents over HTTP. For example,
 * from the Knative event Broker. Try invoking with a request such as this.
 *
 * curl -X POST -d '{"name": "Tiger", "customerId": "0123456789"}' \
 *  -H'Content-type: application/json' \
 *  -H'Ce-id: 1' \
 *  -H'Ce-source: cloud-event-example' \
 *  -H'Ce-type: dev.knative.example' \
 *  -H'Ce-specversion: 1.0' \
 *  http://localhost:8080
 *
 * The CloudEvent object may be accessed via the context parameter, For example:
 *
 * const incomingEvent: CloudEvent = context.cloudevent;
 *
 * Any data in the incoming CloudEvent is extracted and provided as the second
 * parameter to the function.
 *
 * @param {Context} context the invocation context
 * @param {Object} data the CloudEvent data. If the content-type is application/json
 * the data will be converted to an Object via JSON.parse().
 */
export const handle: Invokable = function (
  context: Context,
  cloudevent?: CloudEvent
): Message {
  const meta = {
    source: 'function.eventViewer',
    type: 'echo'
  };
  // The incoming CloudEvent
  if (!cloudevent) {
    const response: CloudEvent = new CloudEvent({
      ...meta,
      ...{ type: 'error', data: 'No event received' }
    });
    context.log.warn(response.toString());
    return HTTP.binary(response);
  }
  // eslint-disable-next-line no-console
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
