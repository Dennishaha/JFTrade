//! Validation helpers for downloaded ADK skill documents.

use std::net::IpAddr;

use reqwest::Url;

pub(super) fn parsed_for_download_host(raw_url: &str) -> Result<String, String> {
    Url::parse(raw_url)
        .map_err(|_| "skill URL is invalid".to_owned())?
        .host_str()
        .map(str::to_owned)
        .ok_or_else(|| "skill URL host is required".to_owned())
}

pub(super) fn unsafe_skill_ip(address: IpAddr) -> bool {
    match address {
        IpAddr::V4(address) => {
            address.is_private()
                || address.is_loopback()
                || address.is_link_local()
                || address.is_broadcast()
                || address.is_unspecified()
                || address.is_multicast()
        }
        IpAddr::V6(address) => {
            address.is_loopback()
                || address.is_unspecified()
                || address.is_multicast()
                || address.is_unique_local()
                || address.is_unicast_link_local()
                || address
                    .to_ipv4_mapped()
                    .is_some_and(|mapped| unsafe_skill_ip(IpAddr::V4(mapped)))
        }
    }
}
