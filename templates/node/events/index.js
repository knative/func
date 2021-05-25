'use strict';
const { CloudEvent, HTTP } = require('cloudevents');

/**
 * A function that responds to incoming CloudEvents over HTTP from,
 * for example, a Knative event Source, Channel or Broker.
 *
 * If running via 'npm run local', it can be invoked like so:
 *
 * curl -X POST -d '{"name": "Tiger", "customerId": "0123456789"}' \
 *  -H'Content-type: application/json' \
 *  -H'Ce-id: 1' \
 *  -H'Ce-source: cloud-event-example' \
 *  -H'Ce-type: dev.knative.example' \
 *  -H'Ce-specversion: 1.0' \
 *  http://localhost:8080
 *
 * @param {Context} context the invocation context
 * @param {Object} event the CloudEvent 
 */
function handle(context, event) {

  context.log.info("context");
  console.log(JSON.stringify(context, null, 2));

  context.log.info("event");
  console.log(JSON.stringify(event, null, 2));

  return HTTP.binary(new CloudEvent({
    source: 'event.handler',
    type: 'echo',
    data: event
  }));
};

module.exports = handle;
