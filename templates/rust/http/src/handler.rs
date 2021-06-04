use actix_web::{http::Method, HttpRequest, HttpResponse};
use log::info;

// Implement your function's logic here
pub async fn index(req: HttpRequest) -> HttpResponse {
    info!("{:#?}", req);
    if req.method() == Method::GET {
        HttpResponse::Ok().body("Hello!\n")
    } else {
        HttpResponse::Ok().body("Thanks!\n")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use actix_web::{body::Body, http, test};

    #[actix_rt::test]
    async fn get() {
        let req = test::TestRequest::get().to_http_request();
        let resp = index(req).await;
        assert_eq!(resp.status(), http::StatusCode::OK);
        assert_eq!(&Body::from("Hello!\n"), resp.body().as_ref().unwrap());
    }

    #[actix_rt::test]
    async fn post() {
        let req = test::TestRequest::post().to_http_request();
        let resp = index(req).await;
        assert!(resp.status().is_success());
        assert_eq!(&Body::from("Thanks!\n"), resp.body().as_ref().unwrap());
    }
}
