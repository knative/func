'use strict';

import test from 'tape';
import { CloudEvent } from 'cloudevents';
import { Context, Invokable } from 'faas-js-runtime';
import * as func from '../build/index.js';

// Ensure that the function completes cleanly when passed a valid event.
test('Unit: handles a valid event', (t) => {
  t.plan(4);
  const data = {
    name: 'tiger',
    customerId: '01234'
  };
  // A valid event includes id, type and source at a minimum.
  const cloudevent: CloudEvent = new CloudEvent({
    id: '01234',
    type: 'com.example.cloudevents.test',
    source: '/test',
    data
  });

  const handle: Invokable = func.handle;
  const mockContext: Context = { cloudevent } as Context;

  // Invoke the function with the valid event, which should complete without error.
  const result = handle(mockContext, cloudevent);
  t.ok(result);
  t.deepEqual(JSON.parse(result.body), data);
  t.equal(result.headers['ce-type'], 'echo');
  t.equal(result.headers['ce-source'], 'function.eventViewer');
  t.end();
});
