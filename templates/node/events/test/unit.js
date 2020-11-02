'use strict';

const func = require('..');
const test = require('tape');
const { CloudEvent } = require('cloudevents');
const { Context } = require('faas-js-runtime/lib/context');

// Ensure that the function completes cleanly when passed a valid event.
test('Unit: handles a valid event', t => {
  t.plan(4);
  const data = {
    name: 'tiger',
    customerId: '01234'
  }
  // A valid event includes id, type and source at a minimum.
  const cloudevent = new CloudEvent({
    id: '01234',
    type: 'com.example.cloudevents.test',
    source: '/test',
    data
  });

  const mockContext = new MockContext(cloudevent);

  // Invoke the function with the valid event, which should compelte without error.
  const result =  func(mockContext, data);
  t.ok(result);
  t.equal(result.body, data);
  t.equal(result.headers['ce-type'], 'user:verified');
  t.equal(result.headers['ce-source'], 'function.verifyUser');
  t.end();
});

class MockContext {
  cloudevent;

  constructor(cloudevent) {
    this.cloudevent = cloudevent;
    this.log = { info: console.log, debug: console.debug }
  }

  cloudEventResponse(data) {
    return new CloudEvent({
      data,
      type: 'com.example.cloudevents.test.response',
      source: '/test'  
    })
  }
}
