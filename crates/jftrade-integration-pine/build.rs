use std::env;
use std::path::{Path, PathBuf};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let manifest_dir = PathBuf::from(env::var("CARGO_MANIFEST_DIR")?);
    let repository_root = manifest_dir
        .parent()
        .and_then(Path::parent)
        .ok_or("integration crate must live below the repository root")?;
    let proto_root = repository_root.join("proto/pineworker");
    let proto = proto_root.join("pineworker.proto");
    let types = proto_root.join("pineworker_types.proto");
    let common = proto_root.join("pineworker_common.proto");

    for input in [&proto, &types, &common] {
        println!("cargo:rerun-if-changed={}", input.display());
    }
    tonic_prost_build::configure()
        .build_server(true)
        .compile_protos(&[proto], &[repository_root.to_path_buf()])?;
    Ok(())
}
