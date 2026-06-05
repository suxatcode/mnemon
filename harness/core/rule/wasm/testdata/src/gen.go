//go:build ignore

// gen.go emits the committed WASM rule modules by hand-encoding the binary directly (no wat2wasm/WABT exists
// in this environment, so this Go encoder stands in for "hand-written WAT → wat2wasm"). The output is a real,
// WASI-free module that imports ONLY env.read_state_view and exports memory/alloc/evaluate. Run once:
//
//	go run ./harness/core/rule/wasm/testdata/src/gen.go
//
// It writes ../rule_allow_if_evidence.wasm (the rule) and ../loop.wasm (an infinite loop, for the deadline
// test). The committed .wasm files mean `go test` needs no toolchain.
package main

import "os"

// ---- LEB128 ----
func uleb(n uint64) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if n == 0 {
			return out
		}
	}
}
func sleb(n int64) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		signBit := b & 0x40
		if (n == 0 && signBit == 0) || (n == -1 && signBit != 0) {
			out = append(out, b)
			return out
		}
		out = append(out, b|0x80)
	}
}

func name(s string) []byte { return append(uleb(uint64(len(s))), []byte(s)...) }

// vec prefixes a sequence of `count` already-concatenated items with their count.
func vec(count int, body []byte) []byte { return append(uleb(uint64(count)), body...) }

func section(id byte, content []byte) []byte {
	return append([]byte{id}, append(uleb(uint64(len(content))), content...)...)
}

// ---- opcode helpers ----
func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
func i32c(v int32) []byte  { return append([]byte{0x41}, sleb(int64(v))...) }
func i64c(v int64) []byte  { return append([]byte{0x42}, sleb(v)...) }
func localGet(i uint64) []byte { return append([]byte{0x20}, uleb(i)...) }
func localSet(i uint64) []byte { return append([]byte{0x21}, uleb(i)...) }
func globalGet(i uint64) []byte { return append([]byte{0x23}, uleb(i)...) }
func globalSet(i uint64) []byte { return append([]byte{0x24}, uleb(i)...) }
func load8(off uint32) []byte  { return append([]byte{0x2d, 0x00}, uleb(uint64(off))...) } // i32.load8_u align=0

const (
	opEnd  = 0x0b
	opAdd  = 0x6a
	opEq   = 0x46
	opGtS  = 0x4a
	opAnd  = 0x71
	opLoop = 0x03
	opBlk  = 0x02
	opIf   = 0x04
	opElse = 0x05
	opBr   = 0x0c
	opBrIf = 0x0d
	opUnreach = 0x00
	tVoid  = 0x40
	tI64   = 0x7e
	tI32   = 0x7f
)

func main() {
	const (
		proposeAt = 1100
		denyAt    = 1400
		bumpStart = 4096
	)
	// propose carries a concrete write so the bridge+kernel can ACCEPT it (and two edges proposing it conflict
	// on m1). deny is the no-evidence path.
	propose := []byte(`{"Verdict":"propose","Proposal":{"Type":"memory.write.proposed","Payload":{"writes":[{"Ref":{"Kind":"memory","ID":"m1"},"Kind":"update","BasedOn":1,"Fields":{"content":"from-wasm"}}]}}}`)
	deny := []byte(`{"Verdict":"deny"}`)
	packed := func(ptr, ln int) int64 { return int64(uint64(ptr)<<32 | uint64(ln)) }
	packedPropose := packed(proposeAt, len(propose))
	packedDeny := packed(denyAt, len(deny))

	// ---- shared sections ----
	typeSec := section(1, vec(3, cat(
		[]byte{0x60}, vec(2, []byte{tI32, tI32}), vec(1, []byte{tI32}), // type0 (i32,i32)->i32  read_state_view
		[]byte{0x60}, vec(1, []byte{tI32}), vec(1, []byte{tI32}), // type1 (i32)->i32  alloc
		[]byte{0x60}, vec(2, []byte{tI32, tI32}), vec(1, []byte{tI64}), // type2 (i32,i32)->i64  evaluate
	)))
	importSec := section(2, vec(1, cat(name("env"), name("read_state_view"), []byte{0x00, 0x00}))) // func type0
	funcSec := section(3, vec(2, []byte{0x01, 0x02}))                                              // alloc:type1, evaluate:type2
	memSec := section(5, vec(1, []byte{0x00, 0x02}))                                               // 1 memory, min 2 pages
	globalSec := section(6, vec(1, cat([]byte{tI32, 0x01}, i32c(bumpStart), []byte{opEnd})))       // mut i32 = 4096
	exportSec := section(7, vec(3, cat(
		name("memory"), []byte{0x02, 0x00},
		name("alloc"), []byte{0x00, 0x01},
		name("evaluate"), []byte{0x00, 0x02},
	)))

	// alloc body: $p=bump; bump+=n; return $p   (locals: 1 i32 = $p at local 1; param n = local 0)
	allocLocals := vec(1, append(uleb(1), tI32))
	allocBody := cat(
		globalGet(0), localSet(1),
		globalGet(0), localGet(0), []byte{opAdd}, globalSet(0),
		localGet(1),
		[]byte{opEnd},
	)
	allocCode := append(uleb(uint64(len(allocLocals)+len(allocBody))), append(allocLocals, allocBody...)...)

	// evaluate body: scan [ptr,ptr+len) for "evidence"; output packed propose/deny.
	// params: ptr=0, len=1 ; locals: i=2, found=3, base=4 (3 i32 locals).
	needle := []byte("evidence")
	var matchExpr []byte
	for k, c := range needle {
		matchExpr = cat(matchExpr, localGet(4), load8(uint32(k)), i32c(int32(c)), []byte{opEq})
		if k > 0 {
			matchExpr = append(matchExpr, opAnd)
		}
	}
	evalLocals := vec(1, append(uleb(3), tI32))
	evalBody := cat(
		[]byte{opBlk, tVoid}, // $done
		[]byte{opLoop, tVoid}, // $outer
		localGet(2), i32c(8), []byte{opAdd}, localGet(1), []byte{opGtS}, []byte{opBrIf, 0x01}, // if i+8>len br $done
		localGet(0), localGet(2), []byte{opAdd}, localSet(4), // base = ptr+i
		matchExpr,
		[]byte{opIf, tVoid}, // if match
		i32c(1), localSet(3), []byte{opBr, 0x02}, // found=1; br $done
		[]byte{opEnd}, // end if
		localGet(2), i32c(1), []byte{opAdd}, localSet(2), // i++
		[]byte{opBr, 0x00}, // br $outer
		[]byte{opEnd}, // end loop
		[]byte{opEnd}, // end block $done
		localGet(3),
		[]byte{opIf, tI64}, i64c(packedPropose), []byte{opElse}, i64c(packedDeny), []byte{opEnd},
		[]byte{opEnd}, // end func
	)
	evalCode := append(uleb(uint64(len(evalLocals)+len(evalBody))), append(evalLocals, evalBody...)...)

	codeSec := section(10, vec(2, cat(allocCode, evalCode)))
	dataSec := section(11, vec(2, cat(
		cat([]byte{0x00}, i32c(proposeAt), []byte{opEnd}, uleb(uint64(len(propose))), propose),
		cat([]byte{0x00}, i32c(denyAt), []byte{opEnd}, uleb(uint64(len(deny))), deny),
	)))

	header := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	rule := cat(header, typeSec, importSec, funcSec, memSec, globalSec, exportSec, codeSec, dataSec)
	write("harness/core/rule/wasm/testdata/rule_allow_if_evidence.wasm", rule)

	// loop.wasm: same shape, but evaluate is an infinite loop (for the deadline test).
	loopLocals := vec(0, nil)
	loopBody := cat([]byte{opLoop, tVoid}, []byte{opBr, 0x00}, []byte{opEnd}, []byte{opUnreach}, []byte{opEnd})
	loopEval := append(uleb(uint64(len(loopLocals)+len(loopBody))), append(loopLocals, loopBody...)...)
	loopCodeSec := section(10, vec(2, cat(allocCode, loopEval)))
	loopMod := cat(header, typeSec, importSec, funcSec, memSec, globalSec, exportSec, loopCodeSec)
	write("harness/core/rule/wasm/testdata/loop.wasm", loopMod)

	// two_imports.wasm: a minimal module importing env.read_state_view AND env.extra — used to prove the
	// promotion import-section check rejects anything beyond the single allowed host import.
	voidType := section(1, vec(1, cat([]byte{0x60}, vec(0, nil), vec(0, nil)))) // type ()->()
	twoImports := section(2, vec(2, cat(
		cat(name("env"), name("read_state_view"), []byte{0x00, 0x00}),
		cat(name("env"), name("extra"), []byte{0x00, 0x00}),
	)))
	write("harness/core/rule/wasm/testdata/two_imports.wasm", cat(header, voidType, twoImports))

	// two_import_sections.wasm: a malformed module with TWO import sections — the first exactly
	// {env.read_state_view}, the second smuggling {env.extra}. Proves the promotion parser does not stop at
	// the first import section (which would let the extra import slip past the gate).
	impA := section(2, vec(1, cat(name("env"), name("read_state_view"), []byte{0x00, 0x00})))
	impB := section(2, vec(1, cat(name("env"), name("extra"), []byte{0x00, 0x00})))
	write("harness/core/rule/wasm/testdata/two_import_sections.wasm", cat(header, voidType, impA, impB))
}

func write(path string, b []byte) {
	if err := os.WriteFile(path, b, 0o644); err != nil {
		panic(err)
	}
	println("wrote", path, len(b), "bytes")
}
