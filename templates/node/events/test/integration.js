'use strict';

const runtime = require('@redhat/faas-js-runtime');
const request = require('supertest');

const func = require('..');
const test = require('tape');
const cloudevents = require('cloudevents-sdk/v1');

const Spec = {
  version: 'ce-specversion',
  type: 'ce-type',
  id: 'ce-id',
  source: 'ce-source'
};

test('Integration: handles a valid event', t => {
  runtime(func, server => {
    t.plan(1);
    request(server)
      .post('/')
      .send({ message: 'hello' })
      .set(Spec.id, 'TEST-EVENT-1')
      .set(Spec.source, 'http://localhost:8080/integration-test')
      .set(Spec.type, 'dev.faas.example')
      .set(Spec.version, '1.0')
      .expect(200)
      .expect('Content-Type', /json/)
      .end((err, res) => {
        t.error(err, 'No error');
        // Check response values that your function produces
        // Uncomment this line to validate the template implementation
        // t.equal(res.body.data.message, 'hello');
        t.end();
        server.close();
      });
  }, { log: false });
});

