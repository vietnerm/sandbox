#!/usr/bin/env bash
# test_alpine.sh — suite cho rootfs Alpine minirootfs (issue #23, GĐ1 mục 2).
#
# Kiểm chứng: Alpine rootfs boot được trong minict giữ nguyên isolation
# (userns/mountns/pidns/netns/capdrop/NNP/seccomp), apk-tools hoạt động
# offline (signature-verified), mô hình ownership single-uid (D20).
#
# AL4 là known-gap TRUNG THỰC (SIA audit PR #24): mount /proc trong mountns
# sau pivot_root+umount old root bị EPERM trên CẢ máy dev lẫn runner (measured
# run 44039937935 — kernel từ chối mount procfs khi không còn procfs "fully
# visible" trong mount ns, fs/proc/root.c mnt_too_revealing). Case kiểu
# "fail-soft vẫn PASS" là RỖNG (vacuous — pass cả khi mount hỏng vì lý do
# khác) nên AL4 giờ assert NGƯỢC LẠI theo pattern PROC1 của suite isolation:
# xfail — /proc KHÔNG có trong sandbox. Khi root-cause được sửa (đề xuất:
# mount proc vào new root TRƯỚC khi umount old root — D20-4), AL4 sẽ đỏ
# FAIL(unexpected success) để nhắc nâng cấp thành assert /proc thật.
#
# AL5 env-aware: nếu CDN/SETUP download fail thì đó KHÔNG phải fail của
# sandbox — case báo PASS(env-blocked) + warning rõ, để CDN outage không làm
# CI đỏ giả (SIA audit PR #24). apk add fail trong sandbox vẫn FAIL thật.
#
# Đếm kỳ vọng: 8 case (AL1–AL8).
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
ROOTFS="$REPO/rootfs-alpine"
MIRROR="${ALPINE_MIRROR:-https://dl-cdn.alpinelinux.org}"
ALPINE_VERSION="${ALPINE_VERSION:-3.22.8}"
BRANCH="v${ALPINE_VERSION%.*}"   # derive URL branch từ version (audit low #4)
ARCH="x86_64"

[[ -x "$MINICT" ]] || { echo "build first: make -C sandbox"; exit 1; }

if [[ ! -x "$ROOTFS/bin/busybox" ]]; then
    bash "$REPO/sandbox/mkrootfs-alpine.sh" "$ROOTFS" >/dev/null || exit 1
fi

# --- pre-seed local repo: 1 gói nhỏ KHÔNG deps (acpid-openrc), version lấy
# từ APKINDEX v3.22 main; signature được apk verify bằng alpine-keys preinstall
# (KHÔNG --allow-untrusted) — CDN + signature = 2 lớp kiểm tra (D20).
PKG="acpid-openrc"
mkdir -p "$ROOTFS/opt/local-repo"
APKINDEX_TGZ="$(mktemp)"
trap 'rm -f "$APKINDEX_TGZ"' EXIT
if curl -fsSL --retry 3 --retry-delay 2 -o "$APKINDEX_TGZ" \
        "$MIRROR/alpine/$BRANCH/main/$ARCH/APKINDEX.tar.gz"; then
    PKG_VER=$(tar -xzOf "$APKINDEX_TGZ" APKINDEX \
        | awk -v p="$PKG" '$0=="P:"p{f=1} f && /^V:/{sub(/^V:/,"");print;exit}')
    curl -fsSL --retry 3 --retry-delay 2 \
        -o "$ROOTFS/opt/local-repo/$PKG.apk" \
        "$MIRROR/alpine/$BRANCH/main/$ARCH/$PKG-${PKG_VER}.apk" \
        || PKG_VER=""
else
    PKG_VER=""
fi

# run a command inside the alpine sandbox
ic() { "$MINICT" "$ROOTFS" /bin/sh -c "$1"; }

# AL4 helper: known-gap /proc (SIA audit PR #24 — honest PROC1 pattern):
# mount /proc in the mountns after pivot_root+umount old root is EPERM on
# BOTH dev machine and runner (measured run 44039937935 — kernel refuses to
# mount procfs when no fully-visible procfs remains in the mount ns,
# fs/proc/root.c mnt_too_revealing). A "fail-soft still PASS" case is
# VACUOUS (passes even when mount fails for unrelated reasons), so AL4 now
# asserts the opposite, PROC1-style: xfail — /proc must be ABSENT while the
# gap exists. xfail semantics = PROC1: the command asserts the FIXED behavior
# (`[ -e /proc/self ]`) and is expected to FAIL for now (rc!=0 ->
# PASS(expected-fail)); once /proc mounts (D20-4 fixed in minict) rc==0 ->
# FAIL(unexpected success) = the upgrade hint to a real /proc assertion.
# When the root cause is fixed in minict (proposal: mount proc into the new
# root BEFORE umount of the old root — D20-4), upgrade this case.
proc4_test() {
    ic '[ -e /proc/self ]'
}
# issue #31 / D39: minict mounts the pidns procfs now; the host probe decides
# which contract applies (same probe as test_isolation.sh)
if unshare --user --map-root-user --pid --fork --mount --mount-proc true \
        >/dev/null 2>&1; then
    PROC_ENV=ok
else
    PROC_ENV=blocked
fi
al4_present_test() {
    ic 'awk "/^Pid:/{p=\$2} END{exit !(p==2)}" /proc/$$/status && [ -r /proc/1/status ]'
}

# AL5 helper: offline apk add, signature-verified (không --allow-untrusted).
# Setup download failure = env-blocked (PASS + visible warning): a CDN outage
# must not paint CI red as a FAKE sandbox failure (SIA audit PR #24); a real
# apk-add failure inside the sandbox still FAILs.
al5_test() {
    if [[ -z "${PKG_VER:-}" ]]; then
        echo "       (env-blocked: APKINDEX/pkg download fail — CDN/mạng;"
        echo "        KHÔNG phải fail của sandbox — SIA audit PR #24)"
        return 0
    fi
    ic "/sbin/apk add --no-network --no-cache --repositories-file /dev/null /opt/local-repo/$PKG.apk >/dev/null 2>&1 && /sbin/apk info -e $PKG"
}

# AL6 helper: ownership model single-uid (D20): file apk cài trong sandbox
# là root:root trong ns, và host-side là uid thật (map 0 <-> uid-thật)
al6_test() {
    ic '[ "$(stat -c %u:%g /etc/init.d/acpid)" = "0:0" ]' || return 1
    [[ "$(stat -c %u "$ROOTFS/etc/init.d/acpid" 2>/dev/null)" == "$(id -u)" ]]
}

# AL7 helper (SIA audit PR #24, low): chown sang uid lạ phải fail với EINVAL
# cụ thể (uid 100 không nằm trong single-uid mapping, D20) — KHÔNG phải fail
# chung chung (expect=fail cũ cũng PASS khi minict không boot được). EINVAL đo
# được 2026-11-17: `chown: /etc/passwd: Invalid argument`, rc=1 — hành vi
# kernel với unmapped uid, giống nhau trên busybox và alpine rootfs.
al7_test() {
    local out
    out="$(ic 'chown 100:100 /etc/passwd 2>&1; echo RC=$?')"
    echo "$out" | grep -q '^RC=1$' || return 1
    echo "$out" | grep -qi 'invalid argument' || return 1
    # sandbox phải còn sống sau chown fail (không chết vì lỗi khác)
    ic '[ -e /etc/passwd ]'
}

PASS=0; FAIL=0
run_case() { # <id> <desc> <expect: pass|fail|xfail> <cmd...>
    local id="$1" desc="$2" expect="$3"; shift 3
    local out rc verdict
    out="$("$@" 2>&1)"; rc=$?
    case "$expect" in
        pass)  [[ $rc -eq 0 ]] && verdict=PASS || verdict=FAIL ;;
        fail)  [[ $rc -ne 0 ]] && verdict=PASS || verdict=FAIL ;;
        xfail) if [[ $rc -ne 0 ]]; then verdict="PASS(expected-fail)"; rc=0;
               else verdict="FAIL(unexpected success)"; fi ;;
    esac
    printf '%-5s %-56s %s\n' "$id" "$desc" "$verdict"
    if [[ "$verdict" == FAIL* ]]; then
        echo "      output: $(echo "$out" | head -3 | tr '\n' ' ')"
        FAIL=$((FAIL+1))
    else
        PASS=$((PASS+1))
    fi
    return 0
}

echo "=== minict alpine-rootfs tests ($(date '+%F %T')) — alpine ${ALPINE_VERSION:-3.22.8}, pkg $PKG=${PKG_VER:-n/a} ==="

run_case AL1 "alpine rootfs boots (busybox alpine build)" pass \
    ic '/bin/busybox | grep -q "BusyBox v" && echo BOOT'
run_case AL2 "sandbox sees OUR passwd marker (not host)" pass \
    ic 'grep -q minict-sandbox /etc/passwd'
run_case AL3 "apk-tools present and runnable in sandbox" pass \
    ic '/sbin/apk --version | grep -q apk-tools'
if [[ $PROC_ENV == ok ]]; then
    run_case AL4 "/proc = pidns view on alpine (payload pid 2) — #31" pass al4_present_test
else
    run_case AL4 "/proc absent: host refuses procfs (env, D39)" xfail proc4_test
fi
run_case AL5 "offline apk add works, signature-verified" pass al5_test
run_case AL6 "single-uid ownership: 0:0 in ns = real uid on host" pass al6_test
run_case AL7 "chown foreign uid: EINVAL (unmapped), sandbox alive" pass al7_test
run_case AL8 "hostname isolated ('minict') on alpine rootfs" pass \
    ic 'hostname | grep -qx minict'

echo "=== summary: PASS=$PASS FAIL=$FAIL ==="
[[ $FAIL -eq 0 ]]
