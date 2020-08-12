"use strict";

const runtime = require('@redhat/faas-js-runtime');
const func = require('./index.js');
const ON_DEATH = require('death')({ uncaughtException: true });

runtime(func, server => {
  ON_DEATH(_ => {
    server.close();
  });

  console.log(`
The server has started.

You can use curl to GET or POST an event to the endpoint:

curl -X GET "http://localhost:8080?name=tiger"

curl -X POST -d '{"name": "tiger"}' "http://localhost:8080" -H'Content-type: application/json'
`);
});
