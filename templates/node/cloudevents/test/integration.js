'use strict';
const { start } = require('faas-js-runtime');
const request = require('supertest');

const func = require('..').handle;
const test = require('tape');

const Spec = {
  version: 'ce-specversion',
  type: 'ce-type',
  id: 'ce-id',
  source: 'ce-source'
};

const data = {
  name: 'tiger',
  customerId: '01234'
}

const errHandler = t => err => {
  t.error(err);
  t.end();
};

test('Integration: handles a valid event', t => {
  start(func).then(server => {
    t.plan(5);
    request(server)
      .post('/')
      .send(data)
      .set(Spec.id, '01234')
      .set(Spec.source, '/test')
      .set(Spec.type, 'com.example.cloudevents.test')
      .set(Spec.version, '1.0')
      .expect(200)
      .expect('Content-Type', /json/)
      .end((err, result) => {
        t.error(err, 'No error');
        t.ok(result);
        t.deepEqual(result.body.data, data);
        t.equal(result.headers['ce-type'], 'echo');
        t.equal(result.headers['ce-source'], 'event.handler');
        t.end();
        server.close();
      });
  }, errHandler(t));
});
