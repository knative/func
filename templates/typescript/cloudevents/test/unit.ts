'use strict';

import test from 'tape';
import { expectType } from 'tsd';
import { CloudEvent } from 'cloudevents';
import { CloudEventFunction, Context } from 'faas-js-runtime';
import { handle, Customer } from '../src';

// Ensure that the function completes cleanly when passed a valid event.
test('Unit: handles a valid event', async (t) => {
  t.plan(4);
  const data: Customer = {
    name: 'tiger',
    customerId: '01234'
  };

  // A valid event includes id, type and source at a minimum.
  const cloudevent: CloudEvent<Customer> = new CloudEvent({
    id: '01234',
    type: 'com.example.cloudevents.test',
    source: '/test',
    data
  });

  // Invoke the function with the valid event, which should complete without error.
  const result = await handle({ log: { info: (_) => _ } } as Context, cloudevent);
  t.ok(result);
  t.deepEqual(result.data, data);
  t.equal(result.type, 'echo');
  t.equal(result.source, 'function.eventViewer');
  t.end();
});

// Ensure that the handle function is typed correctly.
expectType<CloudEventFunction>(handle);
