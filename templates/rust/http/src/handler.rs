use crate::config::HandlerConfig;
use actix_web::{http::Method, web, HttpRequest, HttpResponse};
use log::info;

// Implement your function's logic here
pub async fn index(req: HttpRequest, config: web::Data<HandlerConfig>) -> HttpResponse {
    info!("{:#?}", req);
    if req.method() == Method::GET {
        HttpResponse::Ok().body(format!("Hello {}!\n", config.name))
    } else {
        HttpResponse::Ok().body(format!("Thanks {}!\n", config.name))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use actix_web::{body::to_bytes, http, test, web::Bytes};

    fn config() -> web::Data<HandlerConfig> {
        web::Data::new(HandlerConfig::default())
    }

    #[actix_rt::test]
    async fn get() {
        let req = test::TestRequest::get().to_http_request();
        let resp = index(req, config()).await;
        assert_eq!(resp.status(), http::StatusCode::OK);
        assert_eq!(
            &Bytes::from(format!("Hello {}!\n", "world")),
            to_bytes(resp.into_body()).await.unwrap().as_ref()
        );
    }

    #[actix_rt::test]
    async fn post() {
        let req = test::TestRequest::post().to_http_request();
        let resp = index(req, config()).await;
        assert!(resp.status().is_success());
        assert_eq!(
            &Bytes::from(format!("Thanks {}!\n", "world")),
            to_bytes(resp.into_body()).await.unwrap().as_ref()
        );
    }
}
