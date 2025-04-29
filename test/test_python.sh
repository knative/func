#!/usr/bin/env bash

set -e


if [ "$(go env GOOS)" = "windows" ]; then \
  # Windows-compatible Python tests
  pushd templates/python/http && \
  python -m venv .venv && \
  ./.venv/Scripts/pip install . && \
  ./.venv/Scripts/python -m pytest ./tests && \
  popd

  # Python CloudEvent template tests
  pushd templates/python/cloudevents && \
  python -m venv .venv && \
  ./.venv/Scripts/pip install . && \
  ./.venv/Scripts/python -m pytest ./tests && \
  popd

  # Python Scaffolding Test
  set FUNC_TEST_PYTHON=1 && go test -v ./pkg/oci -run TestBuilder_BuildPython
else \
  #  Python HTTP template tests
  pushd templates/python/http && \
  python -m venv .venv && \
  ./.venv/bin/pip install . && \
  ./.venv/bin/python -m pytest ./tests
  popd

  # Python CloudEvent template tests
  pushd templates/python/cloudevents && \
  python -m venv .venv && \
  ./.venv/bin/pip install . && \
  ./.venv/bin/python -m pytest ./tests
  popd

  # Python Scaffolding Test
  FUNC_TEST_PYTHON=1 go test -v ./pkg/oci -run TestBuilder_BuildPython
fi
