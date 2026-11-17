#!/usr/bin/env bash
# test_alpine.sh — suite cho rootfs Alpine minirootfs (issue #23, GĐ1 mục 2).
#
# Kiểm chứng: Alpine rootfs boot được trong minict giữ nguyên isolation
# (userns/mountns/pidns/netns/capdrop/NNP/seccomp), apk-tools hoạt động
# offline (signature-verified), mô hình ownership single-uid (D20), và
# mount /proc trong pidns — env-aware: máy dev (host seccomp chặn mount)
# assert fail-soft, runner kernel ≥ 6.1 assert /proc thật của pidns.
#
# Đếm kỳ vọng: 8 case (AL1–AL8).
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
MINICT="$REPO/sandbox/minict"
ROOTFS="$REPO/rootfs-alpine"
MIRROR="${ALPINE_MIRROR:-https://dl-cdn.alpinelinux.org}"
BRANCH="v3.22"
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
# same but capture stderr separately (AL4 needs the mount warn)
icc() { "$MINICT" "$ROOTFS" /bin/sh -c "$1" 2>"$2"; }

# AL4 helper: env-aware /proc assertion (D20)
#   dev-host (host seccomp chặn mount — known gap PROC1): minict warn-continue,
#     /proc KHÔNG được mount → assert sandbox vẫn chạy sạch (fail-soft).
#   runner (kernel ≥ 6.1, không filter): procfs mount được vì userns sở hữu
#     pidns (pid_namespaces(7)) → assert /proc/self/status Pid: 1 thật.
proc4_test() {
    local errfile out
    errfile="$(mktemp)"
    out="$(icc '[ -e /proc/self/status ] && grep -q "^Pid:[[:space:]]*1\$" /proc/self/status' "$errfile" 2>/dev/null)"
    local rc=$?
    if grep -q "mount /proc" "$errfile"; then
        rm -f "$errfile"
        # mount bị chặn: phải fail-soft — payload vẫn chạy và exit 0 khối khác
        ic '[ ! -e /proc/self ]'
        return $?
    fi
    rm -f "$errfile"
    return $rc
}

# AL5 helper: offline apk add, signature-verified (không --allow-untrusted)
al5_test() {
    [[ -n "${PKG_VER:-}" ]] || { echo "       (setup: APKINDEX/pkg download fail — KHÔNG phải fail của sandbox)"; return 1; }
    ic "/sbin/apk add --no-network --no-cache --repositories-file /dev/null /opt/local-repo/$PKG.apk >/dev/null 2>&1 && /sbin/apk info -e $PKG"
}

# AL6 helper: ownership model single-uid (D20): file apk cài trong sandbox
# là root:root trong ns, và host-side là uid thật (map 0 <-> uid-thật)
al6_test() {
    ic '[ "$(stat -c %u:%g /etc/init.d/acpid)" = "0:0" ]' || return 1
    [[ "$(stat -c %u "$ROOTFS/etc/init.d/acpid" 2>/dev/null)" == "$(id -u)" ]]
}

PASS=0; FAIL=0
run_case() { # <id> <desc> <expect: pass|fail> <cmd...>
    local id="$1" desc="$2" expect="$3"; shift 3
    local out rc verdict
    out="$("$@" 2>&1)"; rc=$?
    case "$expect" in
        pass) [[ $rc -eq 0 ]] && verdict=PASS || verdict=FAIL ;;
        fail) [[ $rc -ne 0 ]] && verdict=PASS || verdict=FAIL ;;
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
run_case AL4 "/proc in pidns (env-aware: runner true / dev fail-soft)" pass proc4_test
run_case AL5 "offline apk add works, signature-verified" pass al5_test
run_case AL6 "single-uid ownership: 0:0 in ns = real uid on host" pass al6_test
run_case AL7 "chown to foreign uid fails post-capdrop" fail \
    ic 'chown 100:100 /etc/passwd'
run_case AL8 "hostname isolated ('minict') on alpine rootfs" pass \
    ic 'hostname | grep -qx minict'

echo "=== summary: PASS=$PASS FAIL=$FAIL ==="
[[ $FAIL -eq 0 ]]
