/**
 * Unit tests cho files-parse (#208, D79) — chạy bằng node --test
 * (Node ≥ 23.6 type-stripping: import .ts trực tiếp). make console-test.
 * Injection e2e thật qua daemon: MD67 trong tests/test_minictd.sh.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  assertSafeName,
  assertSafePath,
  b64ToBytes,
  bytesToB64,
  decodeBest,
  joinPath,
  parseStatEntries,
  utf8Bytes,
} from "../src/lib/files-parse.ts";

// busybox stat -c "%F\t%s\t%Y\t%a\t%n" — đo thật trên rootfs (MD67 khớp)
const B = "\t";

test("parseStatEntries: busybox rows chuẩn", () => {
  const out = [
    `regular file${B}3${B}1795687538${B}664${B}/probe/a.txt`.slice(7), // name absolute → chỉ tên? KHÔNG: stat %n trả path đầy đủ
  ];
  void out;
});

test("parseStatEntries: rows thật (name = phần cuối, kể cả absolute)", () => {
  const out = [
    `directory${B}4096${B}1795687538${B}775${B}/probe/subdir`,
    `regular file${B}3${B}1795687538${B}664${B}/probe/a.txt`,
    "",
  ].join("\n");
  const rows = parseStatEntries(out);
  assert.equal(rows.length, 2);
  assert.deepEqual(rows[0], { type: "dir", size: 4096, mtime: 1795687538, mode: "775", name: "/probe/subdir" });
  assert.equal(rows[1].type, "file");
  assert.equal(rows[1].name, "/probe/a.txt");
});

test("parseStatEntries: tên file chứa TAB (cột thừa) → name giữ nguyên tab", () => {
  const out = `regular file${B}1${B}1795687538${B}664${B}/probe/with${B}tab.txt`;
  const rows = parseStatEntries(out);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].name, `/probe/with${B}tab.txt`);
});

test("parseStatEntries: dòng lỗi (ít hơn 5 cột) bị bỏ qua", () => {
  const out = [
    `garbage line`,
    `regular file${B}3${B}1795687538${B}664${B}/ok.txt`,
    "x",
  ].join("\n");
  const rows = parseStatEntries(out);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].name, "/ok.txt");
});

test("parseStatEntries: tên file chứa ; $() xuống dòng-cột — không được thành script", () => {
  // tên chứa `; rm -rf /` vẫn chỉ là NAME (argv operand ở adapter — MD67 e2e)
  const out = `regular file${B}2${B}1795687538${B}664${B}/probe/semi;colon$().txt`;
  const rows = parseStatEntries(out);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].name, "/probe/semi;colon$().txt");
});

test("assertSafePath: bắt relative/NUL/quá dài, chấp nhận /", () => {
  assert.throws(() => assertSafePath("etc/passwd"));
  assert.throws(() => assertSafePath("relative"));
  assert.throws(() => assertSafePath("/a\0b"));
  assert.throws(() => assertSafePath("/" + "x".repeat(5000)));
  assert.equal(assertSafePath("/"), "/");
  assert.equal(assertSafePath("/probe/a.txt"), "/probe/a.txt");
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

test("joinPath: root và trailing slash", () => {
  assert.equal(joinPath("/", "a.txt"), "/a.txt");
  assert.equal(joinPath("/probe/", "a.txt"), "/probe/a.txt");
  assert.equal(joinPath("/probe", "a.txt"), "/probe/a.txt");
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
