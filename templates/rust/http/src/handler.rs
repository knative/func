use actix_web::{HttpRequest, HttpResponse};
use log::info;

// Implement your function's logic here
pub async fn index(req: HttpRequest) -> HttpResponse {
    info!("{:#?}", req);
    HttpResponse::Ok().body("OK")
}

#[cfg(test)]
mod tests {
    use super::*;
    use actix_web::{body::to_bytes, http, test::TestRequest, web::Bytes};

    #[actix_rt::test]
    async fn get() {
        let req = TestRequest::get().to_http_request();
        let resp = index(req).await;
        assert_eq!(resp.status(), http::StatusCode::OK);
        assert_eq!(
            &Bytes::from("OK"),
            to_bytes(resp.into_body()).await.unwrap().as_ref()
        );
    }

    #[actix_rt::test]
    async fn post() {
        let req = TestRequest::post().to_http_request();
        let resp = index(req).await;
        assert!(resp.status().is_success());
        assert_eq!(
            &Bytes::from("OK"),
            to_bytes(resp.into_body()).await.unwrap().as_ref()
        );
    }
}
