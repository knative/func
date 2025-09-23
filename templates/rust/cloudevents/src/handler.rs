use crate::config::HandlerConfig;
use actix_web::{error::ErrorInternalServerError, web};
use cloudevents::{event::Data, Event, EventBuilder, EventBuilderV10};
use log::info;
use serde_json::{from_slice, from_str, json};

// Implement your function's logic here
pub async fn handle(
    event: Event,
    config: web::Data<HandlerConfig>,
) -> Result<Event, actix_web::Error> {
    info!("event: {}", event);

    // Extract message from event data, or use default
    let message = match event.data() {
        Some(Data::Binary(v)) => {
            let input: serde_json::Value = from_slice(v)?;
            input.get("message")
                .and_then(|v| v.as_str())
                .unwrap_or(&config.name)
                .to_string()
        },
        Some(Data::String(v)) => {
            let input: serde_json::Value = from_str(v)?;
            input.get("message")
                .and_then(|v| v.as_str())
                .unwrap_or(&config.name)
                .to_string()
        },
        Some(Data::Json(v)) => {
            v.get("message")
                .and_then(|v| v.as_str())
                .unwrap_or(&config.name)
                .to_string()
        },
        None => config.name.clone(),
    };

    EventBuilderV10::from(event)
        .source("func://handler")
        .ty("func.echo")
        .data("application/json", json!({ "message": message }))
        .build()
        .map_err(ErrorInternalServerError)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn config() -> web::Data<HandlerConfig> {
        web::Data::new(HandlerConfig::default())
    }

    #[actix_rt::test]
    async fn valid_input() {
        let mut input = Event::default();
        input.set_data("application/json", json!({"message": "test-echo"}));
        let resp = handle(input, config()).await;
        assert!(resp.is_ok());
        match resp.unwrap().data() {
            Some(Data::Json(output)) => assert_eq!("test-echo", output["message"]),
            _ => panic!(),
        }
    }

    #[actix_rt::test]
    async fn no_input() {
        let resp = handle(Event::default(), config()).await;
        assert!(resp.is_ok());
        match resp.unwrap().data() {
            Some(Data::Json(output)) => assert_eq!("world", output["message"]),
            _ => panic!(),
        }
    }

    #[actix_rt::test]
    async fn invalid_event() {
        use actix_web::{test, web, App};
        let mut app = test::init_service(App::new().route("/", web::post().to(handle))).await;
        let req = test::TestRequest::post().uri("/").to_request();
        let resp = test::call_service(&mut app, req).await;
        assert!(resp.status().is_client_error());
    }
}
