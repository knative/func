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
    use actix_web::{body::Body, http, test};

    fn config() -> web::Data<HandlerConfig> {
        web::Data::new(HandlerConfig::default())
    }

    #[actix_rt::test]
    async fn get() {
        let req = test::TestRequest::get().to_http_request();
        let resp = index(req, config()).await;
        assert_eq!(resp.status(), http::StatusCode::OK);
        assert_eq!(
            &Body::from(format!("Hello {}!\n", "world")),
            resp.body().as_ref().unwrap()
        );
    }

    #[actix_rt::test]
    async fn post() {
        let req = test::TestRequest::post().to_http_request();
        let resp = index(req, config()).await;
        assert!(resp.status().is_success());
        assert_eq!(
            &Body::from(format!("Thanks {}!\n", "world")),
            resp.body().as_ref().unwrap()
        );
    }
}
