'use strict';

const func = require('..');
const test = require('tape');
const { CloudEvent } = require('cloudevents');

// Ensure that the function completes cleanly when passed a valid event.
test('Unit: handles a valid event', t => {
  t.plan(1);
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

  // Invoke the function with the valid event, which should compelte without error.
  t.ok(func(data, { cloudevent, log: { info: console.log } }));
  t.end();
});
