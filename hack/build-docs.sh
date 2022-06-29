#!/usr/bin/env bash

BIN=./func
COMMANDS=("create" "build" "config" "create" "delete" "deploy" "info" "invoke" "list" "repository" "run")
DOC=docs/reference/commands.txt

echo "CLI Command Reference" > ${DOC}
echo >> ${DOC}
echo "------------------------------------------------------------------------------" >> ${DOC}
echo >> ${DOC}

for cmd in ${COMMANDS[@]}; do
  echo ${cmd^^} >> ${DOC}
  echo >> ${DOC}
  $BIN help $cmd >> ${DOC}
  echo >> ${DOC}
  echo "------------------------------------------------------------------------------" >> ${DOC}
  echo >> ${DOC}
done

