use std::env;
use std::path::{Path, PathBuf};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let manifest_dir = PathBuf::from(env::var("CARGO_MANIFEST_DIR")?);
    let repository_root = manifest_dir
        .parent()
        .and_then(Path::parent)
        .ok_or("integration crate must live below the repository root")?;
    let pine_root = repository_root.join("pkg/strategy/pineworker");
    let proto = pine_root.join("proto/pineworker.proto");
    let types = pine_root.join("proto/pineworker_types.proto");
    let common = pine_root.join("proto/pineworker_common.proto");

    for input in [&proto, &types, &common] {
        println!("cargo:rerun-if-changed={}", input.display());
    }
    tonic_prost_build::configure()
        .build_server(true)
        .compile_protos(&[proto], &[pine_root])?;
    Ok(())
}
