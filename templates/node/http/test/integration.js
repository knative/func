'use strict';

const { start } = require('faas-js-runtime');
const request = require('supertest');

const func = require('..');
const test = require('tape');

test('Integration: handles an HTTP GET', t => {
  start(func)
    .then(server => {
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
    });
});

test('Integration: handles an HTTP POST', t => {
  start(func)
    .then(server => {
      t.plan(2);
      request(server)
        .post('/')
        .send({ name: 'tiger' })
        .expect(200)
        .expect('Content-Type', /json/)
        .end((err, res) => {
          t.error(err, 'No error');
          t.deepEqual(res.body, { name: 'tiger' });
          t.end();
          server.close();
        });
    });
});

test('Integration: responds with error code if neither GET or POST', t => {
  start(func)
    .then(server => {
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
    });
});
