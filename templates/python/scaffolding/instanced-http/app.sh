#!/bin/bash

# This file enables S2I support

set -e
exec python service/main.py
