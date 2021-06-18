use actix_web::{web, HttpRequest, HttpResponse};
use cloudevents::{event::Data, EventBuilder, EventBuilderV10};
use cloudevents_sdk_actix_web::{HttpRequestExt, HttpResponseBuilderExt};
use log::info;
use serde_json::json;

// Implement your function's logic here
pub async fn handle(req: HttpRequest, pl: web::Payload) -> Result<HttpResponse, actix_web::Error> {
    let event = req.to_event(pl).await?;
    info!("request: {}", event);

    let input = if let Some(Data::Binary(data)) = event.data() {
        serde_json::from_slice(data).unwrap()
    } else {
        json!({ "name": "world" })
    };

    let response = EventBuilderV10::from(event)
        .source("func://handler")
        .ty("func.example")
        .data("application/json", json!({ "hello": input["name"] }))
        .build()
        .map_err(actix_web::error::ErrorInternalServerError)?;

    info!("response: {}", response);
    HttpResponse::Ok().event(response).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use actix_web::{body::Body, test};

    #[actix_rt::test]
    async fn valid_input() {
        let input = json!({"name": "bootsy"});
        let (req, payload) = test::TestRequest::post()
            .header("ce-specversion", "1.0")
            .header("ce-id", "1")
            .header("ce-type", "test")
            .header("ce-source", "http://localhost")
            .set_json(&input)
            .to_http_parts();
        let resp = handle(req, web::Payload(payload)).await.unwrap();
        assert!(resp.status().is_success());
        assert_eq!(
            &Body::from(json!({"hello":"bootsy"})),
            resp.body().as_ref().unwrap()
        );
    }

    #[actix_rt::test]
    async fn no_input() {
        let (req, payload) = test::TestRequest::post()
            .header("ce-specversion", "1.0")
            .header("ce-id", "1")
            .header("ce-type", "test")
            .header("ce-source", "http://localhost")
            .to_http_parts();
        let resp = handle(req, web::Payload(payload)).await.unwrap();
        assert!(resp.status().is_success());
        assert_eq!(
            &Body::from(json!({"hello":"world"})),
            resp.body().as_ref().unwrap()
        );
    }

    #[actix_rt::test]
    async fn invalid_event() {
        let (req, payload) = test::TestRequest::post().to_http_parts();
        let resp = handle(req, web::Payload(payload)).await;
        assert!(resp.is_err());
    }
}
