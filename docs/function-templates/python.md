# Python Function Developer's Guide

Python Functions allow for the direct deployment of source code as a production
service to any Kubernetes cluster with Knative installed.  The request handler
method signature follows the ASGI (Asynchronous Server Gateway Interface)
standard, allowing for integration with any supporting library.

## Project Structure

When you create a Python function using `func create -l python`, you get a standard Python project structure:

```
❯ func create -l python myfunc
❯ tree myfunc
myfunc/
├── func.yaml           # Function configuration
├── pyproject.toml      # Python project metadata
├── function/
│   ├── __init__.py
│   └── func.py         # Your function implementation
└── tests/
    └── test_func.py    # Unit tests
```

The `func.yaml` file contains build and deployment configuration. For details,
see the [func.yaml reference](../reference/func_yaml.md).

## Function Implementation

Python functions must implement a method `new()` which returns a new instance
of your Function class:

```python
def new():
    """Factory function that returns a Function instance."""
    return Function()

class Function:
    """Your function implementation."""
    pass
```

### Core Methods

Your function class can implement several optional methods:

#### `handle(self, scope, receive, send)`
The main request handler following ASGI protocol. This async method processes all HTTP requests except health checks.

```python
async def handle(self, scope, receive, send):
    """Handle HTTP requests."""
    # Process the request
    await send({
        'type': 'http.response.start',
        'status': 200,
        'headers': [[b'content-type', b'text/plain']],
    })
    await send({
        'type': 'http.response.body',
        'body': b'Hello, World!',
    })
```

#### `start(self, cfg)`

Called when a function instance starts (e.g., during scaling or updates). Receives configuration as a dictionary.

```python
def start(self, cfg):
    """Initialize function with configuration."""
    self.debug = cfg.get('DEBUG', 'false').lower() == 'true'
    logging.info("Function initialized")
```

#### `stop(self)`

Called when a function instance stops. Use for cleanup operations.

```python
def stop(self):
    """Clean up resources."""
    # Close database connections, flush buffers, etc.
    logging.info("Function shutting down")
```

#### `alive(self)` and `ready(self)`

Health check methods exposed at `/health/liveness` and `/health/readiness`:

```python
def alive(self):
    """Liveness check."""
    return True, "Function is alive"

def ready(self):
    """Readiness check."""
    if self.database_connected:
        return True, "Ready to serve"
    return False, "Database not connected"
```

## Local Development

### Running Your Function

```bash
# Build and run on the host (not in a container)
func run --builder=host

# Force rebuild even if no changes detected
func run --build
```

### Testing

Test your function with HTTP requests:

```bash
# Test the main endpoint
curl http://localhost:8080

# Check health endpoints
curl http://localhost:8080/health/liveness
curl http://localhost:8080/health/readiness
```

### Testing CloudEvent Functions

Create a CloudEvent function:

```bash
# Create a new CloudEvent function
func create -l python -t cloudevents myeventfunc
```

Test CloudEvent functions using curl with proper headers:

```bash
# Invoke with a CloudEvent
curl -X POST http://localhost:8080 \
  -H "Ce-Specversion: 1.0" \
  -H "Ce-Type: com.example.sampletype" \
  -H "Ce-Source: example/source" \
  -H "Ce-Id: 1234-5678-9101" \
  -H "Ce-Subject: example-subject" \
  -H "Content-Type: application/json" \
  -d '{"message": "Hello CloudEvent!"}'
```

Also see `func invoke` which automates this for basic testing.

### Unit Testing

Python functions use modern Python packaging with `pyproject.toml` and include
pytest with async support for testing ASGI functions. The generated project
includes example tests in `tests/test_func.py` that demonstrate how to test the
async handler.

#### Setting Up Your Development Environment

It's best practice to use a virtual environment to isolate your function's dependencies:

```bash
# Create a virtual environment (Python 3.3+)
python3 -m venv venv

# Activate the virtual environment
# On Linux/macOS:
source venv/bin/activate
# On Windows:
# venv\Scripts\activate

# Upgrade pip to ensure you have the latest version
python -m pip install --upgrade pip

# Install the function package and its dependencies (including test dependencies)
pip install -e .

# Run tests with pytest
pytest

# Run tests with verbose output
pytest -v

# Run tests with coverage (requires pytest-cov)
pip install pytest-cov
pytest --cov=function --cov-report=term-missing

# When done, deactivate the virtual environment
deactivate
```

**Note**:
- Python 3 typically comes with `venv` module built-in
- If `python3` command is not found, try `python` instead
- The `-m pip` syntax ensures you're using the pip from your virtual environment
- Always activate your virtual environment before running tests or installing dependencies

#### Writing Tests for ASGI Functions

The test file demonstrates how to test ASGI functions by mocking the ASGI interface:

```python
import pytest
from function import new

@pytest.mark.asyncio
async def test_function_handle():
    # Create function instance
    f = new()

    # Mock ASGI scope (request details)
    scope = {
        'type': 'http',
        'method': 'POST',
        'path': '/',
        'headers': [(b'content-type', b'application/json')],
    }

    # Mock receive callable (for request body)
    async def receive():
        return {
            'type': 'http.request',
            'body': b'{"test": "data"}',
            'more_body': False,
        }

    # Track sent responses
    responses = []

    # Mock send callable
    async def send(message):
        responses.append(message)

    # Call the handler
    await f.handle(scope, receive, send)

    # Assert responses
    assert len(responses) == 2
    assert responses[0]['type'] == 'http.response.start'
    assert responses[0]['status'] == 200
    assert responses[1]['type'] == 'http.response.body'
```

#### Testing CloudEvent Functions

For CloudEvent functions, include CloudEvent headers in your test scope:

```python
@pytest.mark.asyncio
async def test_cloudevent_handler():
    f = new()

    # CloudEvent headers
    scope = {
        'type': 'http',
        'method': 'POST',
        'path': '/',
        'headers': [
            (b'ce-specversion', b'1.0'),
            (b'ce-type', b'com.example.test'),
            (b'ce-source', b'test/unit'),
            (b'ce-id', b'test-123'),
            (b'content-type', b'application/json'),
        ],
    }

    # Test with CloudEvent data
    async def receive():
        return {
            'type': 'http.request',
            'body': b'{"message": "test event"}',
            'more_body': False,
        }

    # ... rest of test
```

#### Testing with Real HTTP Clients

For integration testing, you can use httpx with ASGI support:

```python
import httpx
import pytest
from function import new

@pytest.mark.asyncio
async def test_with_http_client():
    f = new()

    # Create ASGI transport with your function
    transport = httpx.ASGITransport(app=f.handle)

    # Make HTTP requests
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.get("/")
        assert response.status_code == 200

        response = await client.post("/", json={"test": "data"})
        assert response.status_code == 200
```

## Advanced Implementation Examples

### Processing Request Data

```python
async def handle(self, scope, receive, send):
    """Process POST requests with JSON data."""
    if scope['method'] == 'POST':
        # Receive request body
        body = b''
        while True:
            message = await receive()
            if message['type'] == 'http.request':
                body += message.get('body', b'')
                if not message.get('more_body', False):
                    break

        # Process JSON data
        import json
        data = json.loads(body)
        result = process_data(data)

        # Send response
        response_body = json.dumps(result).encode()
        await send({
            'type': 'http.response.start',
            'status': 200,
            'headers': [[b'content-type', b'application/json']],
        })
        await send({
            'type': 'http.response.body',
            'body': response_body,
        })
```

### Environment-Based Configuration

```python
class Function:
    def start(self, cfg):
        """Configure function from environment."""
        self.api_key = cfg.get('API_KEY')
        self.cache_ttl = int(cfg.get('CACHE_TTL', '300'))
        self.log_level = cfg.get('LOG_LEVEL', 'INFO')

        logging.basicConfig(level=self.log_level)
```

### CloudEvent Handling

For CloudEvent support, parse the headers and body accordingly:

```python
async def handle(self, scope, receive, send):
    """Handle CloudEvents."""
    headers = dict(scope['headers'])

    # Check if this is a CloudEvent
    if b'ce-type' in headers:
        event_type = headers[b'ce-type'].decode()
        event_source = headers[b'ce-source'].decode()
        # Process CloudEvent...
```

## Deployment

### Basic Deployment

```bash
# Deploy to a specific registry
func deploy --builder=host --registry docker.io/myuser
```


For all deploy options, see `func deploy --help`
