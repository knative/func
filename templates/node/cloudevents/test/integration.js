'use strict';
const { HTTP, CloudEvent } = require('cloudevents');
const { start } = require('faas-js-runtime');
const request = require('supertest');

const func = require('..').handle;
const test = require('tape');

const data = {
  name: 'tiger',
  customerId: '01234'
}

const errHandler = t => err => {
  t.error(err);
  t.end();
};

const message = HTTP.binary(new CloudEvent({
  type: 'com.example.test',
  source: 'http://localhost:8080',
  data
}));

test('Integration: handles a valid event', t => {
  start(func).then(server => {
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
        t.deepEqual(result.body, data);
        t.equal(result.headers['ce-type'], 'echo');
        t.equal(result.headers['ce-source'], 'event.handler');
        t.end();
        server.close();
      });
  }, errHandler(t));
});
