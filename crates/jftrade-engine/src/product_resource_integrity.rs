fn current_executable_sha256() -> Result<String, ProductError> {
    let path = env::current_exe().map_err(ProductError::CurrentExecutable)?;
    let file = File::open(&path).map_err(|source| ProductError::ReadExecutable {
        path: path.clone(),
        source,
    })?;
    let mut reader = BufReader::new(file);
    let mut buffer = [0_u8; 64 * 1024];
    let mut digest = Sha256::new();
    loop {
        let count = reader
            .read(&mut buffer)
            .map_err(|source| ProductError::ReadExecutable {
                path: path.clone(),
                source,
            })?;
        if count == 0 {
            break;
        }
        digest.update(&buffer[..count]);
    }
    Ok(encode_sha256(digest.finalize()))
}

fn encode_sha256(digest: impl IntoIterator<Item = u8>) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut encoded = String::with_capacity(64);
    for byte in digest {
        encoded.push(char::from(HEX[usize::from(byte >> 4)]));
        encoded.push(char::from(HEX[usize::from(byte & 0x0f)]));
    }
    encoded
}
