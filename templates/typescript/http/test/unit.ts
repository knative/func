'use strict';

import test from 'tape';
import { Context } from 'faas-js-runtime';
import * as func from '../build/index.js';

// Ensure that the function completes cleanly when passed a valid event.
test('Unit: handles a valid request', async (t) => {
  t.plan(2);
  const body: Record<string, string> = {
    name: 'tiger',
    customerId: '01234'
  };

  const handle = func.handle;
  const mockContext = { body } as Context;

  // Invoke the function which should complete without error and echo the data
  const result = await handle(mockContext);
  t.ok(result);
  t.equal(result, JSON.stringify(body));
  t.end();
});
