'use strict';

import test from 'tape';
import { expectType } from 'tsd';
import { CloudEvent } from 'cloudevents';
import { CloudEventFunction, Context } from 'faas-js-runtime';
import { handle } from '../src';

// Ensure that the function completes cleanly when passed a valid event.
test('Unit: handles a valid event', async (t) => {
  t.plan(4);
  const data = {
    name: 'tiger',
    customerId: '01234'
  };

  // A valid event includes id, type and source at a minimum.
  const cloudevent = new CloudEvent({
    id: '01234',
    type: 'com.example.cloudevents.test',
    source: '/test',
    data
  });

  // Invoke the function with the valid event, which should complete without error.
  const result = await handle({ log: { info: (_) => _ } } as Context, cloudevent);
  t.ok(result);
  t.deepEqual(result.data, { message: 'OK' });
  t.equal(result.type, 'function.response');
  t.equal(result.source, 'function');
  t.end();
});

// Ensure that the handle function is typed correctly.
expectType<CloudEventFunction>(handle);
