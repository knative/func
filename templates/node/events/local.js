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

You can use curl to POST an event to the endpoint:

curl -X POST -d '{"hello": "world"}' \\
    -H'Content-type: application/json' \\
    -H'Ce-id: 1' \\
    -H'Ce-source: cloud-event-example' \\
    -H'Ce-type: dev.knative.example' \\
    -H'Ce-specversion: 1.0' \\
    http://localhost:8080
  `);
});
