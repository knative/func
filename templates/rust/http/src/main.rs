use actix_web::{web, App, HttpResponse, HttpServer};
use env_logger as elog;

mod config;
mod handler;

#[actix_web::main]
async fn main() -> std::io::Result<()> {
    elog::Builder::from_env(elog::Env::default().default_filter_or("info")).init();

    let port: u16 = match std::env::var("PORT") {
        Ok(v) => v.parse().unwrap(),
        Err(_) => 8080,
    };

    // Create the HTTP server
    HttpServer::new(|| {
        App::new()
            .wrap(actix_web::middleware::Logger::default())
            .configure(config::configure)
            .route("/", web::get().to(handler::index))
            .route("/", web::post().to(handler::index))
            .route(
                "/health/{_:(readiness|liveness)}",
                web::get().to(HttpResponse::Ok),
            )
    })
    .bind(("127.0.0.1", port))?
    .workers(1)
    .run()
    .await
}
