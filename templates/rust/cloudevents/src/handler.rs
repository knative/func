use actix_web::error::ErrorInternalServerError;
use cloudevents::{Event, EventBuilder, EventBuilderV10};
use log::info;

// Implement your function's logic here
pub async fn handle(event: Event) -> Result<Event, actix_web::Error> {
    info!("event: {}", event);

    EventBuilderV10::from(event)
        .source("function")
        .ty("function.response")
        .data("application/json", r#"{"message":"OK"}"#)
        .build()
        .map_err(ErrorInternalServerError)
}

#[cfg(test)]
mod tests {
    use super::*;
    use cloudevents::event::Data;

    #[actix_rt::test]
    async fn returns_ok() {
        let resp = handle(Event::default()).await;
        assert!(resp.is_ok());
        match resp.unwrap().data() {
            Some(Data::String(output)) => assert_eq!(r#"{"message":"OK"}"#, output),
            _ => panic!("Expected string data"),
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
