pub const ABI_VERSION: &str = "mnemon-wasm-rule-v0";

#[repr(C)]
pub struct PackedSlice {
    pub ptr: u32,
    pub len: u32,
}

pub fn pack(ptr: u32, len: u32) -> u64 {
    ((ptr as u64) << 32) | (len as u64)
}

pub fn unpack(value: u64) -> PackedSlice {
    PackedSlice {
        ptr: (value >> 32) as u32,
        len: value as u32,
    }
}

pub fn alloc_bytes(len: usize) -> *mut u8 {
    let mut buf = Vec::<u8>::with_capacity(len);
    let ptr = buf.as_mut_ptr();
    core::mem::forget(buf);
    ptr
}
