use crate::BacktestError;
use crate::model::BacktestOutput;

pub(crate) fn populate_result_hash(output: &mut BacktestOutput) -> Result<(), BacktestError> {
    output.result_hash.clear();
    let bytes = serde_json::to_vec(output)?;
    let mut hash = 0xcbf2_9ce4_8422_2325_u64;
    for byte in bytes {
        hash ^= u64::from(byte);
        hash = hash.wrapping_mul(0x0000_0100_0000_01b3);
    }
    output.result_hash = format!("fnv1a64:{hash:016x}");
    Ok(())
}
