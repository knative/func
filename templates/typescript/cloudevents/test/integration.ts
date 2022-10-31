'use strict';
import { CloudEvent, HTTP } from 'cloudevents';
import { start, InvokerOptions } from 'faas-js-runtime';
import request from 'supertest';

import * as func from '../build';
import test, { Test } from 'tape';

// Test typed CloudEvent data
interface Customer {
  name: string;
  customerId: string;
}

const data: Customer = {
  name: 'tiger',
  customerId: '01234'
};

const message = HTTP.binary(
  new CloudEvent({
    source: '/test/integration',
    type: 'test',
    data
  })
);

const errHandler = (t: Test) => (err: Error) => {
  t.error(err);
  t.end();
};

test('Integration: handles a valid event', (t) => {
  start(func.handle, {} as InvokerOptions).then((server) => {
    t.plan(5);
    request(server)
      .post('/')
      .send(message.body)
      .set(message.headers)
      .expect(200)
      .expect('Content-Type', /json/)
      .end((err, result) => {
        t.error(err, 'No error');
        t.ok(result);
        t.deepEqual(result.body, JSON.parse(message.body as string));
        t.equal(result.headers['ce-type'], 'echo');
        t.equal(result.headers['ce-source'], 'function.eventViewer');
        t.end();
        server.close();
      });
  }, errHandler(t));
});
