'use strict';
const { CloudEvent } = require('cloudevents');
const runtime = require('faas-js-runtime');
const request = require('supertest');

const func = require('..');
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

test('Integration: handles a valid event', t => {
  runtime(func, server => {
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
        t.deepEqual(result.body, data);
        t.equal(result.headers['ce-type'], 'user:verified');
        t.equal(result.headers['ce-source'], 'function.verifyUser');
        t.end();
        server.close();
      });
  }, { log: false });
});

