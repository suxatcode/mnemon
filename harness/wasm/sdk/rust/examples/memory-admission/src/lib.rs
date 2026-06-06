use mnemon_wasm_sdk::{alloc_bytes, pack};

#[no_mangle]
pub extern "C" fn alloc(len: u32) -> u32 {
    alloc_bytes(len as usize) as u32
}

#[no_mangle]
pub extern "C" fn evaluate(ptr: u32, len: u32) -> u64 {
    let input = unsafe { core::slice::from_raw_parts(ptr as *const u8, len as usize) };
    let decision = if contains(input, b"evidence") {
        br#"{"Verdict":"propose","Proposal":{"Type":"memory.write.proposed"}}"#
    } else {
        br#"{"Verdict":"deny"}"#
    };
    let out = alloc_bytes(decision.len());
    unsafe {
        core::ptr::copy_nonoverlapping(decision.as_ptr(), out, decision.len());
    }
    pack(out as u32, decision.len() as u32)
}

fn contains(haystack: &[u8], needle: &[u8]) -> bool {
    haystack.windows(needle.len()).any(|window| window == needle)
}
