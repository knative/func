#!/usr/bin/env bash

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

# Get the script's directory (test/)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Get the project root (one level up from test/)
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Detect Python executable path (Windows vs Unix)
get_python_path() {
  if [[ -f ".venv/Scripts/python.exe" ]]; then
    echo ".venv/Scripts/python.exe"
  else
    echo ".venv/bin/python"
  fi
}

# Test HTTP Template
cd "${PROJECT_ROOT}/templates/python/http"

# Create virtual environment
python3 -m venv .venv || python -m venv .venv

# Get the correct Python path for this platform
PYTHON_PATH=$(get_python_path)

# Install and run tests (no activation needed)
"${PYTHON_PATH}" -m pip install -q --upgrade pip
"${PYTHON_PATH}" -m pip install -q -e .
"${PYTHON_PATH}" -m pytest -v

# Cleanup
rm -rf .venv

echo "✓ Python HTTP template tests passed"

# Test CloudEvents Template
cd "${PROJECT_ROOT}/templates/python/cloudevents"

# Create virtual environment
python3 -m venv .venv || python -m venv .venv

# Get the correct Python path for this platform
PYTHON_PATH=$(get_python_path)

# Install and run tests (no activation needed)
"${PYTHON_PATH}" -m pip install -q --upgrade pip
"${PYTHON_PATH}" -m pip install -q -e .
"${PYTHON_PATH}" -m pytest -v

# Cleanup
rm -rf .venv

echo "✓ All Python template tests completed successfully"
