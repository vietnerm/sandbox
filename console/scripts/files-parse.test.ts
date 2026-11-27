/**
 * Unit tests cho files-parse (#208 D79 + #219 D83) — chạy bằng node --test
 * (Node ≥ 23.6 type-stripping: import .ts trực tiếp). make console-test.
 * Injection e2e thật qua daemon: MD67 (operand argv) + MD71 (bytes/b64
 * wire) trong tests/test_minictd.sh.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  FS_DECODE_SNIPPET,
  FS_LIST_SCRIPT_B64,
  assertSafeName,
  assertSafePathB64,
  b64ToBytes,
  b64ToText,
  bytesToB64,
  crumbsFromB64,
  decodeBest,
  decodeFsName,
  encodePathB64,
  joinPathB64,
  parentB64,
  parseStatEntries,
  utf8Bytes,
} from "../src/lib/files-parse.ts";

// busybox list rows: `type\tsize\tmtime\tmode\tpath_b64\tname_b64` (#219/D83)
const B = "\t";
const b64 = (s: string) => bytesToB64(utf8Bytes(s));

test("parseStatEntries: rows chuẩn (6 cột, b64 round-trip)", () => {
  const out = [
    ["directory", "4096", "1795687538", "775", b64("/probe/subdir"), b64("subdir")].join(B),
    ["regular file", "3", "1795687538", "664", b64("/probe/a.txt"), b64("a.txt")].join(B),
    "",
  ].join("\n");
  const rows = parseStatEntries(out);
  assert.equal(rows.length, 2);
  assert.deepEqual(rows[0], {
    type: "dir", size: 4096, mtime: 1795687538, mode: "775",
    path_b64: b64("/probe/subdir"), name_b64: b64("subdir"),
  });
  assert.equal(rows[1].type, "file");
  assert.equal(new TextDecoder().decode(b64ToBytes(rows[1].name_b64)), "a.txt");
});

test("parseStatEntries: tên chứa NEWLINE/TAB không còn vỡ hàng (b64 gỡ hạn chế v1)", () => {
  // raw name bytes: "a\n\tb;notcmd.txt" — base64 không chứa tab/newline nên row nguyên vẹn
  const nameBytes = utf8Bytes("a\n\tb;notcmd.txt");
  const out = [
    ["regular file", "5", "1795760320", "664", bytesToB64(utf8Bytes("/probe219/a\n\tb;notcmd.txt")), bytesToB64(nameBytes)].join(B),
  ].join("\n");
  const rows = parseStatEntries(out);
  assert.equal(rows.length, 1);
  assert.deepEqual(Array.from(b64ToBytes(rows[0].name_b64)), Array.from(nameBytes));
});

test("parseStatEntries: tên byte LỖI (t\\xe1\\xbaext.txt) giữ nguyên byte qua b64", () => {
  const raw = [0x74, 0xe1, 0xba, 0x65, 0x78, 0x74]; // "t" + invalid + "ext"
  const out = [["regular file", "5", "1795760320", "664", bytesToB64(new Uint8Array([0x2f, ...raw])), bytesToB64(new Uint8Array(raw))].join(B)].join("\n");
  const rows = parseStatEntries(out);
  assert.equal(rows.length, 1);
  assert.deepEqual(Array.from(b64ToBytes(rows[0].name_b64)), raw);
});

test("parseStatEntries: dòng lỗi / b64 rác bị bỏ qua", () => {
  const out = [
    "garbage",
    ["regular file", "3", "1795687538", "664", b64("/ok.txt"), b64("ok.txt")].join(B),
    ["regular file", "3", "1795687538", "664", "!!!notb64!!!", b64("x")].join(B),
    ["regular file", "3", "1795687538", "664", b64("/two.txt")].join(B), // 5 cột
  ].join("\n");
  const rows = parseStatEntries(out);
  assert.equal(rows.length, 1);
  assert.deepEqual(Array.from(b64ToBytes(rows[0].name_b64)), Array.from(utf8Bytes("ok.txt")));
});

test("decodeFsName: tên hợp lệ → text; byte lỗi → placeholder + hex (#219)", () => {
  const ok = decodeFsName(b64("tiếng.txt"));
  assert.equal(ok.display, "tiếng.txt");
  assert.equal(ok.valid, true);
  assert.equal(ok.hex, "");

  // t + 0xE1 + 0xBA + "ext.txt" — ví dụ trong issue #219 (hex: 74 e1 ba 65 78 74)
  const bad = decodeFsName(bytesToB64(new Uint8Array([0x74, 0xe1, 0xba, 0x65, 0x78, 0x74])));
  assert.equal(bad.valid, false);
  assert.equal(bad.display, "<tên không phải UTF-8>");
  assert.equal(bad.hex, "hex: 74 e1 ba 65 78 74");
});

test("decodeFsName: emoji 😀 và combining é vẫn là UTF-8 hợp lệ", () => {
  assert.equal(decodeFsName(b64("😀.txt")).valid, true);
  assert.equal(decodeFsName(b64("é.txt")).valid, true); // NFD combining
});

test("assertSafePathB64: guard trên BYTES — relative/NUL/quá dài bị chặn, tên lỗi vẫn hợp lệ", () => {
  assert.throws(() => assertSafePathB64(b64("etc/passwd")));
  assert.throws(() => assertSafePathB64(bytesToB64(new Uint8Array([0x61, 0])))); // "a\0"
  assert.throws(() => assertSafePathB64(""));
  assert.throws(() => assertSafePathB64(b64("/" + "x".repeat(5000))));
  // path có newline + byte lỗi VẪN hợp lệ (đúng chủ đích #219)
  const weird = bytesToB64(new Uint8Array([0x2f, 0x70, 0x0a, 0xe1, 0xba]));
  assert.equal(assertSafePathB64(weird), weird);
  assert.equal(assertSafePathB64(b64("/probe/a.txt")), b64("/probe/a.txt"));
  // root "/" hợp lệ (delete guard nằm ở adapter, không phải ở path guard)
  assert.equal(assertSafePathB64(b64("/")), b64("/"));
});

test("assertSafeName: bắt '', '.', '..', 'a/b', backslash, NUL", () => {
  assert.throws(() => assertSafeName(""));
  assert.throws(() => assertSafeName("."));
  assert.throws(() => assertSafeName(".."));
  assert.throws(() => assertSafeName("a/b"));
  assert.throws(() => assertSafeName("a\\b"));
  assert.throws(() => assertSafeName("a\0b"));
  assert.equal(assertSafeName("a.txt"), "a.txt");
  assert.equal(assertSafeName("semi;colon$().txt"), "semi;colon$().txt");
});

test("joinPathB64: nối BYTE level, dir chứa byte lỗi không round-trip qua string", () => {
  const dirB64 = bytesToB64(new Uint8Array([0x2f, 0x70, 0xe1, 0xba])); // "/p\xe1\xba"
  const got = b64ToBytes(joinPathB64(dirB64, "a.txt"));
  assert.deepEqual(Array.from(got), [0x2f, 0x70, 0xe1, 0xba, 0x2f, 0x61, 0x2e, 0x74, 0x78, 0x74]);
  assert.equal(new TextDecoder().decode(b64ToBytes(joinPathB64(b64("/"), "x"))), "/x");
  assert.equal(new TextDecoder().decode(b64ToBytes(joinPathB64(b64("/probe/"), "x"))), "/probe/x");
});

test("parentB64: byte level, root tự về root", () => {
  assert.equal(new TextDecoder().decode(b64ToBytes(parentB64(b64("/a/b")))), "/a");
  assert.equal(new TextDecoder().decode(b64ToBytes(parentB64(b64("/a")))), "/");
  assert.equal(new TextDecoder().decode(b64ToBytes(parentB64(b64("/")))), "/");
  const weirdDir = bytesToB64(new Uint8Array([0x2f, 0x61, 0x2f, 0xe1, 0xba]));
  assert.deepEqual(Array.from(b64ToBytes(parentB64(weirdDir))), [0x2f, 0x61]);
});

test("crumbsFromB64: tách segment ở byte level + label best-effort", () => {
  const cs = crumbsFromB64(b64("/probe/tiếng.txt"));
  assert.equal(cs.length, 2);
  assert.equal(cs[0].label, "probe");
  assert.equal(cs[1].label, "tiếng.txt");
  assert.equal(cs[1].valid, true);
  // segment byte lỗi → placeholder, valid=false
  const bad = crumbsFromB64(bytesToB64(new Uint8Array([0x2f, 0x74, 0xe1, 0xba, 0x65, 0x78, 0x74])));
  assert.equal(bad.length, 1);
  assert.equal(bad[0].valid, false);
  assert.equal(bad[0].display ?? bad[0].label, "<tên không phải UTF-8>");
});

test("encodePathB64/b64ToText: chuỗi UTF-8 hợp lệ round-trip; b64ToText bắt byte lỗi", () => {
  assert.equal(b64ToText(encodePathB64("/probe/x.txt")), "/probe/x.txt");
  assert.throws(() => b64ToText(bytesToB64(new Uint8Array([0xe1, 0xba]))));
});

test("bytesToB64/b64ToBytes roundtrip binary + utf8", () => {
  const bytes = new Uint8Array([0, 1, 2, 250, 251, 65, 66, 10]);
  assert.deepEqual(Array.from(b64ToBytes(bytesToB64(bytes))), Array.from(bytes));
  const s = "hello; $(inject) — tiếng Việt";
  assert.equal(new TextDecoder().decode(b64ToBytes(bytesToB64(utf8Bytes(s)))), s);
});

test("decodeBest: utf8 hợp lệ → text; binary → base64", () => {
  assert.deepEqual(decodeBest(utf8Bytes("xuân")), { content: "xuân", encoding: "utf8" });
  const bin = new Uint8Array([0xff, 0xfe, 0x00, 0x80]);
  const r = decodeBest(bin);
  assert.equal(r.encoding, "base64");
  assert.deepEqual(Array.from(b64ToBytes(r.content)), Array.from(bin));
});

test("scripts là HẰNG SỐ decode-b64: mọi op script bắt đầu bằng decode snippet", () => {
  // guard chống ai đó vô tình interpolation path vào script text
  assert.ok(FS_LIST_SCRIPT_B64.startsWith(FS_DECODE_SNIPPET));
  assert.ok(FS_LIST_SCRIPT_B64.includes('printf %s "$1" | base64 -d'));
  assert.ok(!FS_LIST_SCRIPT_B64.includes("${1}")); // operand không bao giờ vào thân script
  for (const s of [FS_LIST_SCRIPT_B64]) {
    // mọi script list phải kết thúc bằng `true` (stat race không fail listing)
    assert.ok(s.endsWith("done; true"));
  }
});
