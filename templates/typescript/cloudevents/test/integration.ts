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
      .buffer(true)
      .parse((res, callback) => {
        let data = '';
        res.on('data', (chunk: string) => {
          data += chunk;
        });
        res.on('end', () => callback(null, data));
      })
      .end((err, result) => {
        t.error(err, 'No error');
        t.ok(result);
        t.equal(result.body, 'OK');
        t.equal(result.headers['ce-type'], 'function.response');
        t.equal(result.headers['ce-source'], 'function');
        t.end();
        server.close();
      });
  }, errHandler(t));
});
