/**
 * Handles an incoming HTTP GET
 * @param {Context} context a context object containing the HTTP request data
 */
function handleGet(context) {
  return {
    query: context.query,
    name: context.query.name,
  }
};

/**
 * Handles an incoming HTTP POST
 * @param {Context} context a context object containing the HTTP request data
 */
function handlePost(context) {
  return {
    body: context.body,
    name: context.body.name
  }
};

/**
 * An HTTP handling function which reads the HTTP request
 * method from the context object and invokes the appropriate
 * request handler.
 * @param {Context} context a context object containing the HTTP request data
 */
function invoke(context) {
  context.log.info(`Handling HTTP ${context.httpVersion} request`);
  console.log(JSON.stringify(context, null, 2));
  if (context.method === 'POST') {
    return handlePost(context);
  } else if (context.method === 'GET') {
    return handleGet(context);
  } else {
    return { statusCode: 451, statusMessage: 'Unavailable for Legal Reasons' };
  }
}

/**
 * If you don't need any of the HTTP information from the
 * context instance, you may choose to have your function
 * invoked with destructured query parameters. For example,
 * this function expects a URL such as:
 *
 * curl -X GET localhost:8080?name=tiger
 *
 * @param {string} name the "name" query parameter
 */
function invokeDestructured({ name }) {
  return `Hello ${name}!`;
}

module.exports = invoke;
