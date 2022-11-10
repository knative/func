'use strict';

const func = require('..').handle;
const test = require('tape');
const { CloudEvent } = require('cloudevents');

// Ensure that the function completes cleanly when passed a valid event.
test('Unit: handles a valid event', async t => {
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

  // Invoke the function with the valid event, which should complete without error.
  const result = await func({ log: { info: (_) => _ } }, cloudevent);
  t.ok(result);
  t.equal(result.data, data);
  t.equal(result.type, 'echo');
  t.equal(result.source, 'event.handler');
  t.end();
});
