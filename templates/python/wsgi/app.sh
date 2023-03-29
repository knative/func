#!/bin/sh

exec gunicorn func:main --bind=0.0.0.0:8080 --access-logfile=-
