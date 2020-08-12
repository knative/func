'use strict';

const func = require('..');
const test = require('tape');
const { CloudEvent } = require('cloudevents');

// Ensure that the function completes cleanly when passed a valid event.
test('Unit: handles a valid event', t => {
  t.plan(1);
  // A valid event includes id, type and source at a minimum.
  const cloudevent = new CloudEvent({
    id: 'TEST-EVENT-01',
    type: 'com.example.cloudevents.test',
    source: '/test',
    data: {
      message: 'hello'
    }
  });

  // Invoke the function with the valid event, which should compelte without error.
  t.ok(func({ cloudevent, log: { info: console.log } }));
  t.end();
});
