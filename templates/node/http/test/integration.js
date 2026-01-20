'use strict';

const { start } = require('faas-js-runtime');
const request = require('supertest');

const func = require('..').handle;
const test = require('tape');

const errHandler = t => err => {
  t.error(err);
  t.end();
};

test('Integration: handles an HTTP GET', t => {
  start(func).then(server => {
    t.plan(2);
    request(server)
      .get('/?name=tiger')
      .expect(200)
      .end((err, res) => {
        t.error(err, 'No error');
        t.equal(res.text, 'OK');
        t.end();
        server.close();
      });
  }, errHandler(t));
});

test('Integration: handles an HTTP POST', t => {
  start(func).then(server => {
    t.plan(2);
    request(server)
      .post('/')
      .send({ name: 'tiger' })
      .expect(200)
      .end((err, res) => {
        t.error(err, 'No error');
        t.equal(res.text, 'OK');
        t.end();
        server.close();
      });
  }, errHandler(t));
});
