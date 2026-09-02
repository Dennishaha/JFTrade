pub(super) const fn side_label(value: i32) -> &'static str {
    match value {
        1 => "BUY",
        2 => "SELL",
        3 => "SELL_SHORT",
        4 => "BUY_BACK",
        _ => "UNKNOWN",
    }
}

pub(super) const fn order_type_label(value: i32) -> &'static str {
    match value {
        1 => "LIMIT",
        2 => "MARKET",
        3 => "STOP",
        4 => "STOP_LIMIT",
        5 => "ABSOLUTE_LIMIT",
        6 => "AUCTION",
        7 => "AUCTION_LIMIT",
        _ => "UNKNOWN",
    }
}
