'use strict';

const func = require('..');
const test = require('tape');

const fixture = { log: { info: console.log } };

test('Unit: handles an HTTP GET', t => {
  t.plan(1);
  // Invoke the function, which should compelte without error.
  const result = func({ ...fixture, method: 'GET', query: { name: 'tiger' } });
  t.deepEqual(result, { query: { name: 'tiger' }, name: 'tiger' });
  t.end();
});

test('Unit: handles an HTTP POST', t => {
  t.plan(1);
  // Invoke the function, which should compelte without error.
  const result = func({ ...fixture, method: 'POST', body: { name: 'tiger' } });
  t.deepEqual(result, { body: { name: 'tiger' }, name: 'tiger' });
  t.end();
});

test('Unit: responds with error code if neither GET or POST', t => {
  t.plan(1);
  const result = func(fixture);
  t.deepEqual(result, { statusCode: 451, statusMessage: 'Unavailable for Legal Reasons' });
  t.end();
});
