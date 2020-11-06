'use strict';
const { CloudEvent, HTTP } = require('cloudevents');

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
 * The event data is extracted from the incoming event and provided as the first
 * parameter to the function. The CloudEvent object itself may be accessed via the
 * context parameter, For example:
 *
 * const incomingEvent = context.cloudevent;
 *
 * @param {Context} context the invocation context
 * @param {Object} user the CloudEvent data. If the data content type is application/json
 * this will be converted to an Object via JSON.parse()
 */
function verifyUser(context, user) {
  if (!context.cloudevent) {
    return {
      message: 'No cloud event received'
    };
  }

  context.log.info('Processing user', user);
  context.log.info(`CloudEvent received: ${context.cloudevent.toString()}`);

  user = verify(user);
  return HTTP.binary(new CloudEvent({
    source: 'function.verifyUser',
    type: 'user:verified',
    data: user
  }));
};

function verify(user) {
  // do something with the user
  return user;
}

module.exports = verifyUser;
