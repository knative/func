use actix_web::{web, App, HttpResponse, HttpServer};
use env_logger::{Builder, Env};
use std::{
    env,
    io::{Error, ErrorKind, Result},
    num::ParseIntError,
};

mod config;
mod handler;

#[actix_web::main]
async fn main() -> Result<()> {
    Builder::from_env(Env::default().default_filter_or("info,actix_web=warn")).init();

    let port = match env::var("PORT") {
        Ok(v) => v
            .parse()
            .map_err(|e: ParseIntError| Error::new(ErrorKind::Other, e.to_string()))?,
        Err(_) => 8080,
    };

    // Create the HTTP server
    HttpServer::new(|| {
        App::new()
            .wrap(actix_web::middleware::Logger::default())
            .configure(config::configure)
            .route("/", web::post().to(handler::handle))
            .route("/", web::get().to(HttpResponse::MethodNotAllowed))
            .route(
                "/health/{_:(readiness|liveness)}",
                web::get().to(HttpResponse::Ok),
            )
    })
    .bind(("0.0.0.0", port))?
    .workers(1)
    .run()
    .await
}
