const { CloudEvent, HTTP } = require('cloudevents');

/**
 * Your CloudEvent handling function, invoked with each request.
 * This example function logs its input, and responds with a CloudEvent
 * which echoes the incoming event data
 *
 * It can be invoked with 'func emit'
 *
 * @param {Context} context a context object.
 * @param {object} context.body the request body if any
 * @param {object} context.query the query string deserialzed as an object, if any
 * @param {object} context.log logging object with methods for 'info', 'warn', 'error', etc.
 * @param {object} context.headers the HTTP request headers
 * @param {string} context.method the HTTP request method
 * @param {string} context.httpVersion the HTTP protocol version
 * See: https://github.com/knative-sandbox/kn-plugin-func/blob/main/docs/guides/nodejs.md#the-context-object
 * @param {CloudEvent} event the CloudEvent
 */
function handle(context, event) {
  context.log.info("context");
  context.log.info(JSON.stringify(context, null, 2));

  context.log.info("event");
  context.log.info(JSON.stringify(event, null, 2));

  return HTTP.binary(new CloudEvent({
    source: 'event.handler',
    type: 'echo',
    data: event
  }));
};

module.exports = { handle };
