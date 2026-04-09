use wasi::exports::wasi::http::incoming_handler::Guest;
use wasi::http::proxy::export;
use wasi::http::types::{Fields, IncomingRequest, OutgoingBody, OutgoingResponse, ResponseOutparam};

export!(Function);

pub struct Function;

impl Guest for Function {
    fn handle(request: IncomingRequest, response_out: ResponseOutparam) {
        let path = request
            .path_with_query()
            .unwrap_or_else(|| "/".to_string());

        // Delegate to the testable core function.
        let (status, content_type, body) = process(&path);

        let headers = Fields::new();
        headers
            .set(&"content-type".to_string(), &[content_type.into_bytes()])
            .unwrap();

        let response = OutgoingResponse::new(headers);
        response.set_status_code(status).unwrap();

        let out_body = response.body().unwrap();
        let stream = out_body.write().unwrap();
        stream.blocking_write_and_flush(body.as_bytes()).unwrap();
        drop(stream);

        OutgoingBody::finish(out_body, None).unwrap();
        ResponseOutparam::set(response_out, Ok(response));
    }
}

/// Core function logic: given a request path, return (status, content-type, body).
///
/// YOUR CODE HERE
///
/// Note: `IncomingRequest` is a host-owned WIT resource with no guest-side
/// constructor — it cannot be instantiated in tests without a full WASI host.
/// Keeping real logic here lets you unit-test it with plain Rust values, which
/// is the recommended pattern for WASI HTTP guest components.
fn process(path: &str) -> (u16, String, String) {
    let body = format!("Hello from WASI! Path: {path}\n");
    (200, "text/plain; charset=utf-8".to_string(), body)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_process_root() {
        let (status, content_type, body) = process("/");
        assert_eq!(status, 200);
        assert!(content_type.contains("text/plain"));
        assert!(body.contains("Hello from WASI!"));
        assert!(body.contains("Path: /"));
    }

    #[test]
    fn test_process_path_with_query() {
        let (status, _, body) = process("/greet?name=world");
        assert_eq!(status, 200);
        assert!(body.contains("Path: /greet?name=world"));
    }
}
