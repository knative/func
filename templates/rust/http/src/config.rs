use actix_web::web::ServiceConfig;
use log::info;

/// Run custom configuration as part of the application building
/// process.
pub fn configure(_cfg: &mut ServiceConfig) {
    info!("Configuring service");
}
