pub(super) const fn side_label(value: i32) -> &'static str {
    match value {
        1 => "BUY",
        2 => "SELL",
        3 => "SELL_SHORT",
        4 => "BUY_BACK",
        _ => "UNKNOWN",
    }
}

/// Maps an OpenD `Trd_Common.OrderType` wire code back to the neutral order
/// type carried by the execution ledger.
///
/// The code set must mirror the parser in
/// `product_production_ports_execution_order_parse.rs`; Go keeps the same
/// round trip through `trdOrderTypeFromBrokerOrderType`, which normalizes
/// `Stop`/`StopLimit` back to `STOP`/`STOP_LIMIT` before comparison. Keeping a
/// single neutral vocabulary on both the write and the broker-snapshot path is
/// what lets `matches_safe_attributes` correlate a submitted order with its
/// OpenD snapshot.
pub(super) const fn order_type_label(value: i32) -> &'static str {
    match value {
        1 => "LIMIT",
        2 => "MARKET",
        5 => "ABSOLUTE_LIMIT",
        6 => "AUCTION",
        7 => "AUCTION_LIMIT",
        8 => "SPECIAL_LIMIT",
        9 => "SPECIAL_LIMIT_ALL",
        10 => "STOP",
        11 => "STOP_LIMIT",
        12 => "TAKE_PROFIT_MARKET",
        13 => "TAKE_PROFIT",
        14 => "TRAILING_STOP_MARKET",
        15 => "TRAILING_STOP_LIMIT",
        _ => "UNKNOWN",
    }
}
