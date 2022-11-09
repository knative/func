'use strict';

import test from 'tape';
import { expectType } from 'tsd';
import { Context, HTTPFunction } from 'faas-js-runtime';
import { handle } from '../build/index.js';

// Ensure that the function completes cleanly when passed a valid event.
test('Unit: handles a valid request', async (t) => {
  t.plan(2);
  const body: Record<string, string> = {
    name: 'tiger',
    customerId: '01234'
  };

  // Invoke the function which should complete without error and echo the data
  const result = await handle({ log: { info: (_) => _ } } as Context, body);
  t.ok(result);
  t.equal(result.body, body);
  t.end();
});

// Ensure that the handle function is typed correctly.
expectType<HTTPFunction>(handle);
