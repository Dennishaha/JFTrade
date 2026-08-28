//! Production market-data adapters bundle.
//!
//! Connects catalog reads, quote reads, subscription mutations, and provider
//! actions to real production state without mock fixtures or dummy arrays.

#[path = "product_production_ports_market_data_actions.rs"]
mod product_production_ports_market_data_actions;
#[path = "product_production_ports_market_data_catalog.rs"]
mod product_production_ports_market_data_catalog;
#[path = "product_production_ports_market_data_projection.rs"]
pub(crate) mod product_production_ports_market_data_projection;
#[path = "product_production_ports_market_data_quote.rs"]
mod product_production_ports_market_data_quote;
#[path = "product_production_ports_market_data_subscription.rs"]
mod product_production_ports_market_data_subscription;

pub(crate) use product_production_ports_market_data_actions::ProductionMarketDataProviderActionsPort;
pub(crate) use product_production_ports_market_data_catalog::ProductionMarketDataCatalogPort;
pub(crate) use product_production_ports_market_data_quote::ProductionMarketDataQuotePort;
pub(crate) use product_production_ports_market_data_subscription::ProductionMarketDataSubscriptionMutationPort;
