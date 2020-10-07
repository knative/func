'use strict';

const runtime = require('faas-js-runtime');
const request = require('supertest');

const func = require('..');
const test = require('tape');

test('Integration: handles an HTTP GET', t => {
  runtime(func, server => {
    t.plan(2);
    request(server)
      .get('/?name=tiger')
      .expect(200)
      .expect('Content-Type', /json/)
      .end((err, res) => {
        t.error(err, 'No error');
        t.deepEqual(res.body, { query: { name: 'tiger' }, name: 'tiger' });
        t.end();
        server.close();
      });
  }, { log: false });
});

test('Integration: handles an HTTP POST', t => {
  runtime(func, server => {
    t.plan(2);
    request(server)
      .post('/')
      .send({ name: 'tiger' })
      .expect(200)
      .expect('Content-Type', /json/)
      .end((err, res) => {
        t.error(err, 'No error');
        t.deepEqual(res.body, { body: { name: 'tiger' }, name: 'tiger' });
        t.end();
        server.close();
      });
  }, { log: false });
});

test('Integration: responds with error code if neither GET or POST', t => {
  runtime(func, server => {
    t.plan(1);
    request(server)
      .put('/')
      .send({ name: 'tiger' })
      .expect(200)
      .expect('Content-Type', /json/)
      .end((err, res) => {
        t.deepEqual(res.body, { message: 'Route PUT:/ not found', error: 'Not Found', statusCode: 404 });
        t.end();
        server.close();
      });
  }, { log: false });
});
