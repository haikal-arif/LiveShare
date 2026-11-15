use std::net::SocketAddr;

use server::config::Config;

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt::init();

    let config = Config::load();
    let bind_addr = config.bind_addr.clone();
    let (app, rooms) = server::build(config);

    tracing::info!("sfu listening on {bind_addr}");
    let listener = tokio::net::TcpListener::bind(&bind_addr)
        .await
        .expect("failed to bind sfu address");
    // Connect info gives peers' socket addresses, the fallback when the
    // reverse proxy sets no `X-Real-IP`.
    axum::serve(
        listener,
        app.into_make_service_with_connect_info::<SocketAddr>(),
    )
    .with_graceful_shutdown(async move {
        shutdown_signal().await;
        tracing::info!("shutting down: closing all rooms");
        rooms.close_all().await;
    })
    .await
    .expect("sfu server crashed");
}

/// Resolves on Ctrl-C or, on unix, SIGTERM (what an orchestrator sends).
async fn shutdown_signal() {
    let ctrl_c = async {
        let _ = tokio::signal::ctrl_c().await;
    };

    #[cfg(unix)]
    let terminate = async {
        match tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate()) {
            Ok(mut signal) => {
                signal.recv().await;
            }
            Err(_) => std::future::pending::<()>().await,
        }
    };
    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}
