use cloudevents::{event::Data, Event, EventBuilder, EventBuilderV10};
use log::info;
use serde_json::{from_slice, from_str, json};

// Implement your function's logic here
pub async fn handle(event: Event) -> Result<Event, actix_web::Error> {
    info!("event: {}", event);

    let input = match event.data() {
        Some(Data::Binary(v)) => from_slice(v)?,
        Some(Data::String(v)) => from_str(v)?,
        Some(Data::Json(v)) => v.to_owned(),
        None => json!({ "name": "world" }),
    };

    EventBuilderV10::from(event)
        .source("func://handler")
        .ty("func.example")
        .data("application/json", json!({ "hello": input["name"] }))
        .build()
        .map_err(actix_web::error::ErrorInternalServerError)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[actix_rt::test]
    async fn valid_input() {
        let mut input = Event::default();
        input.set_data("application/json", json!({"name": "bootsy"}));
        let resp = handle(input).await;
        assert!(resp.is_ok());
        match resp.unwrap().data() {
            Some(Data::Json(output)) => assert_eq!("bootsy", output["hello"]),
            _ => panic!(),
        }
    }

    #[actix_rt::test]
    async fn no_input() {
        let resp = handle(Event::default()).await;
        assert!(resp.is_ok());
        match resp.unwrap().data() {
            Some(Data::Json(output)) => assert_eq!("world", output["hello"]),
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
