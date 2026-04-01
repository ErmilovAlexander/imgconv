#!/usr/bin/env bash
set -Eeuo pipefail

###############################################################################
# imgconv full regression script
#
# This version:
#   - does NOT build
#   - does NOT run go test
#   - runs only against an existing imgconv binary
#   - works from /var/tmp by default
#   - targets these inputs under --root:
#       * qcow2 file: Rocky-9-GenericCloud.latest.x86_64.qcow2
#       * vdi directory: vdi_calculate
#       * vmdk directory: vmdk_calculate
###############################################################################

ROOT_IMAGES=""
WORKDIR="/var/tmp/imgconv_full_regress"
IMGCONV_BIN=""
THREADS="${THREADS:-4}"
CHUNK_MIB="${CHUNK_MIB:-4}"
KEEP_WORK=0

timestamp() {
  date '+%Y-%m-%d %H:%M:%S'
}

log() {
  printf '[%s] %s\n' "$(timestamp)" "$*"
}

fail() {
  printf '[%s] ERROR: %s\n' "$(timestamp)" "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

usage() {
  cat <<'EOF'
Usage:
  full_regress_imgconv_runtime.sh --root <dir> --bin <imgconv_binary> [options]

Required:
  --root <dir>         Root directory containing:
                         - Rocky-9-GenericCloud.latest.x86_64.qcow2
                         - vdi_calculate/
                         - vmdk_calculate/
  --bin <path>         Path to existing imgconv binary

Options:
  --workdir <dir>      Working directory (default: /var/tmp/imgconv_full_regress)
  --threads <n>        Threads for convert (default: 4)
  --chunk-mib <n>      Chunk size in MiB (default: 4)
  --keep-work          Keep workdir on success
  -h, --help           Show help

Example:
  ./full_regress_imgconv_runtime.sh \
    --root ~/Downloads \
    --bin /var/tmp/imgconv
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root) ROOT_IMAGES="$2"; shift 2 ;;
    --bin) IMGCONV_BIN="$2"; shift 2 ;;
    --workdir) WORKDIR="$2"; shift 2 ;;
    --threads) THREADS="$2"; shift 2 ;;
    --chunk-mib) CHUNK_MIB="$2"; shift 2 ;;
    --keep-work) KEEP_WORK=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ -n "${ROOT_IMAGES}" ]] || fail "--root is required"
[[ -d "${ROOT_IMAGES}" ]] || fail "root directory not found: ${ROOT_IMAGES}"
[[ -n "${IMGCONV_BIN}" ]] || fail "--bin is required"
[[ -x "${IMGCONV_BIN}" ]] || fail "imgconv binary not found or not executable: ${IMGCONV_BIN}"

need_cmd qemu-img
need_cmd cmp
need_cmd sha256sum

mkdir -p "${WORKDIR}"
LOG_DIR="${WORKDIR}/logs"
OUT_DIR="${WORKDIR}/outputs"
RAW_CACHE_DIR="${WORKDIR}/raw_cache"
mkdir -p "${LOG_DIR}" "${OUT_DIR}" "${RAW_CACHE_DIR}"

SUMMARY_OK="${WORKDIR}/summary_ok.txt"
SUMMARY_FAIL="${WORKDIR}/summary_fail.txt"
: > "${SUMMARY_OK}"
: > "${SUMMARY_FAIL}"

cleanup() {
  if [[ "${KEEP_WORK}" -eq 0 && -f "${WORKDIR}/.success" ]]; then
    rm -rf "${WORKDIR}/case_tmp" 2>/dev/null || true
  fi
}
trap cleanup EXIT

detect_fmt() {
  local p="$1"
  local base="${p##*/}"
  local lower="${base,,}"
  case "${lower}" in
    *.qcow2) echo "qcow2" ;;
    *.vmdk) echo "vmdk" ;;
    *.vdi) echo "vdi" ;;
    *.raw|*.img|*.bin) echo "raw" ;;
    *) echo "" ;;
  esac
}

safe_name() {
  local p="$1"
  local s="${p//\//__}"
  s="${s// /_}"
  s="${s//(/_}"
  s="${s//)/_}"
  s="${s//:/_}"
  echo "${s}"
}

source_to_raw() {
  local src="$1"
  local fmt="$2"
  local out="$3"
  case "${fmt}" in
    raw)
      cp --sparse=always "${src}" "${out}"
      ;;
    qcow2|vmdk|vdi)
      qemu-img convert -f "${fmt}" -O raw "${src}" "${out}"
      ;;
    *)
      fail "unsupported source format for raw export: ${fmt}"
      ;;
  esac
}

output_ext_for_fmt() {
  case "$1" in
    raw) echo "raw" ;;
    qcow2) echo "qcow2" ;;
    vdi) echo "vdi" ;;
    *) fail "unsupported output fmt: $1" ;;
  esac
}

run_info_and_check() {
  local src="$1"
  local fmt="$2"
  local name="$3"
  local logf="${LOG_DIR}/${name}_inspect.log"
  : > "${logf}"

  echo "CMD: ${IMGCONV_BIN} info -i ${src} --input-format ${fmt}" >>"${logf}"
  "${IMGCONV_BIN}" info -i "${src}" --input-format "${fmt}" >>"${logf}" 2>&1

  case "${fmt}" in
    qcow2|vmdk|vdi)
      echo "CMD: ${IMGCONV_BIN} check -i ${src} --input-format ${fmt}" >>"${logf}"
      "${IMGCONV_BIN}" check -i "${src}" --input-format "${fmt}" >>"${logf}" 2>&1
      ;;
  esac

  echo "CMD: qemu-img info ${src}" >>"${logf}"
  qemu-img info "${src}" >>"${logf}" 2>&1 || true
  case "${fmt}" in
    qcow2)
      echo "CMD: qemu-img check ${src}" >>"${logf}"
      qemu-img check "${src}" >>"${logf}" 2>&1 || true
      ;;
  esac
}

compare_raws() {
  local a="$1"
  local b="$2"
  local logf="$3"

  cmp "${a}" "${b}" >>"${logf}" 2>&1
  sha256sum "${a}" "${b}" >>"${logf}" 2>&1
}

test_conversion_case() {
  local src="$1"
  local infmt="$2"
  local outfmt="$3"

  local case_id
  case_id="$(safe_name "${src}")__to__${outfmt}"
  local case_dir="${OUT_DIR}/${case_id}"
  local logf="${LOG_DIR}/${case_id}.log"
  mkdir -p "${case_dir}"
  : > "${logf}"

  local ext
  ext="$(output_ext_for_fmt "${outfmt}")"
  local out="${case_dir}/out.${ext}"
  local src_raw="${RAW_CACHE_DIR}/${case_id}_src.raw"
  local out_raw="${RAW_CACHE_DIR}/${case_id}_out.raw"

  {
    echo "SOURCE=${src}"
    echo "INPUT_FORMAT=${infmt}"
    echo "OUTPUT_FORMAT=${outfmt}"
    echo "CMD: ${IMGCONV_BIN} convert -i ${src} -o ${out} --input-format ${infmt} --format ${outfmt} --threads ${THREADS} --chunk-mib ${CHUNK_MIB} --sparse --verify full"
  } >>"${logf}"

  "${IMGCONV_BIN}" convert \
    -i "${src}" \
    -o "${out}" \
    --input-format "${infmt}" \
    --format "${outfmt}" \
    --threads "${THREADS}" \
    --chunk-mib "${CHUNK_MIB}" \
    --sparse \
    --verify full >>"${logf}" 2>&1

  "${IMGCONV_BIN}" info -i "${out}" --input-format "${outfmt}" >>"${logf}" 2>&1

  case "${outfmt}" in
    qcow2|vdi)
      "${IMGCONV_BIN}" check -i "${out}" --input-format "${outfmt}" >>"${logf}" 2>&1
      ;;
  esac

  qemu-img info "${out}" >>"${logf}" 2>&1 || true
  case "${outfmt}" in
    qcow2)
      qemu-img check "${out}" >>"${logf}" 2>&1
      ;;
    vdi)
      qemu-img check "${out}" >>"${logf}" 2>&1 || true
      ;;
  esac

  source_to_raw "${src}" "${infmt}" "${src_raw}"
  source_to_raw "${out}" "${outfmt}" "${out_raw}"
  compare_raws "${src_raw}" "${out_raw}" "${logf}"

  echo "${case_id}" >> "${SUMMARY_OK}"
}

test_conversion_case_wrapper() {
  local src="$1"
  local infmt="$2"
  local outfmt="$3"
  local name
  name="$(safe_name "${src}")__to__${outfmt}"

  log "TEST conversion ${name}"
  if ! test_conversion_case "${src}" "${infmt}" "${outfmt}"; then
    echo "${name}" >> "${SUMMARY_FAIL}"
    log "FAILED conversion ${name}"
    return 1
  fi
  return 0
}

run_feature_suite() {
  local feat_dir="${WORKDIR}/feature_suite"
  local feat_log="${LOG_DIR}/feature_suite.log"
  mkdir -p "${feat_dir}"
  : > "${feat_log}"

  local base="${feat_dir}/base.qcow2"
  local overlay="${feat_dir}/overlay.qcow2"
  local flat="${feat_dir}/flat.qcow2"
  local base_raw="${feat_dir}/base.raw"
  local overlay_raw="${feat_dir}/overlay.raw"
  local flat_raw="${feat_dir}/flat.raw"
  local map_json="${feat_dir}/map.json"
  local measure_json="${feat_dir}/measure.json"

  log "RUN feature suite"

  {
    echo "== create base"
    "${IMGCONV_BIN}" create -f qcow2 -o "${base}" --size 64M

    echo "== seed base"
    qemu-img convert -f qcow2 -O raw "${base}" "${base_raw}"
    printf '\x41%.0s' {1..4096} >/dev/null 2>&1 || true
    qemu-img convert -f qcow2 -O qcow2 "${base}" "${base}" >/dev/null 2>&1 || true
    python3 - <<'PY'
from pathlib import Path
p = Path(r"__BASE_RAW__")
data = bytearray(p.read_bytes())
data[0:1024*1024] = b"\x41"*(1024*1024)
data[16*1024*1024:17*1024*1024] = b"\x42"*(1024*1024)
p.write_bytes(data)
PY
  } >>"${feat_log}" 2>&1 || true

  # Replace placeholder path and continue with deterministic operations.
  python3 - <<PY
from pathlib import Path
p = Path(r"${feat_log}")
txt = p.read_text()
txt = txt.replace("__BASE_RAW__", r"${base_raw}")
p.write_text(txt)
PY

  {
    echo "== rebuild base with deterministic raw payload"
    qemu-img convert -f raw -O qcow2 "${base_raw}" "${base}.new"
    mv -f "${base}.new" "${base}"

    echo "== create overlay"
    (
      cd "${feat_dir}"
      "${IMGCONV_BIN}" create -f qcow2 -o "${overlay}" --size 64M --backing-file "$(basename "${base}")"
    )

    echo "== make flat overlay from base then modify through imgconv chain checks later"
    qemu-img convert -f qcow2 -O raw "${overlay}" "${overlay_raw}"
    python3 - <<PY
from pathlib import Path
p = Path(r"${overlay_raw}")
data = bytearray(p.read_bytes())
data[4*1024*1024:5*1024*1024] = b"\x55"*(1024*1024)
p.write_bytes(data)
PY
    qemu-img convert -f raw -O qcow2 "${overlay_raw}" "${overlay}.new"
    mv -f "${overlay}.new" "${overlay}"
    "${IMGCONV_BIN}" rebase -i "${overlay}" --backing-file "$(basename "${base}")"

    echo "== overlay info"
    "${IMGCONV_BIN}" info -i "${overlay}" --json

    echo "== map overlay"
    "${IMGCONV_BIN}" map -i "${overlay}" --json > "${map_json}"
    cat "${map_json}"

    echo "== measure qcow2"
    "${IMGCONV_BIN}" measure -f qcow2 --size 500G --cluster-bits 16 --json > "${measure_json}"
    cat "${measure_json}"

    echo "== flatten overlay"
    "${IMGCONV_BIN}" convert \
      -i "${overlay}" \
      -o "${flat}" \
      --format qcow2 \
      --threads "${THREADS}" \
      --chunk-mib "${CHUNK_MIB}" \
      --sparse \
      --verify full

    echo "== compare overlay vs flat"
    "${IMGCONV_BIN}" compare -a "${overlay}" -b "${flat}" --mode full --chunk-mib "${CHUNK_MIB}"

    echo "== qemu-img checks"
    qemu-img check "${base}"
    qemu-img check "${overlay}"
    qemu-img check "${flat}"

    echo "== raw compare overlay vs flat"
    qemu-img convert -O raw "${overlay}" "${overlay_raw}"
    qemu-img convert -O raw "${flat}" "${flat_raw}"
    cmp "${overlay_raw}" "${flat_raw}"

    echo "== commit overlay"
    "${IMGCONV_BIN}" commit -i "${overlay}" --chunk-mib "${CHUNK_MIB}"

    echo "== compare overlay vs base after commit"
    "${IMGCONV_BIN}" compare -a "${overlay}" -b "${base}" --mode full --chunk-mib "${CHUNK_MIB}"

    echo "== raw compare base vs overlay after commit"
    qemu-img convert -O raw "${base}" "${base_raw}"
    qemu-img convert -O raw "${overlay}" "${overlay_raw}"
    cmp "${base_raw}" "${overlay_raw}"

    echo "== rebase overlay path"
    (
      cd "${feat_dir}"
      "${IMGCONV_BIN}" rebase -i "${overlay}" --backing-file "$(basename "${base}")"
    )
    "${IMGCONV_BIN}" info -i "${overlay}" --json

  } >>"${feat_log}" 2>&1 || {
    echo "feature_suite" >> "${SUMMARY_FAIL}"
    log "FAILED feature suite (see ${feat_log})"
    return 1
  }

  echo "feature_suite" >> "${SUMMARY_OK}"
  return 0
}

find_qcow2_target() {
  find "${ROOT_IMAGES}" -type f -name 'Rocky-9-GenericCloud.latest.x86_64.qcow2' | head -n1
}

find_first_in_dir_by_ext() {
  local dir="$1"
  local ext="$2"
  find "${dir}" -type f -name "*.${ext}" | sort | head -n1
}

log "ROOT_IMAGES=${ROOT_IMAGES}"
log "WORKDIR=${WORKDIR}"
log "IMGCONV_BIN=${IMGCONV_BIN}"
log "THREADS=${THREADS}"
log "CHUNK_MIB=${CHUNK_MIB}"

QCOW2_SRC="$(find_qcow2_target)"
[[ -n "${QCOW2_SRC}" ]] || fail "qcow2 source Rocky-9-GenericCloud.latest.x86_64.qcow2 not found under ${ROOT_IMAGES}"

VDI_DIR="${ROOT_IMAGES}/vdi_calculate"
[[ -d "${VDI_DIR}" ]] || fail "directory not found: ${VDI_DIR}"
VDI_SRC="$(find_first_in_dir_by_ext "${VDI_DIR}" "vdi")"
[[ -n "${VDI_SRC}" ]] || fail "no .vdi file found in ${VDI_DIR}"

VMDK_DIR="${ROOT_IMAGES}/vmdk_calculate"
[[ -d "${VMDK_DIR}" ]] || fail "directory not found: ${VMDK_DIR}"
VMDK_SRC="$(find_first_in_dir_by_ext "${VMDK_DIR}" "vmdk")"
[[ -n "${VMDK_SRC}" ]] || fail "no .vmdk file found in ${VMDK_DIR}"

declare -a INPUTS=("${QCOW2_SRC}" "${VDI_SRC}" "${VMDK_SRC}")

log "Selected inputs:"
for src in "${INPUTS[@]}"; do
  log "  - ${src}"
done

for src in "${INPUTS[@]}"; do
  fmt="$(detect_fmt "${src}")"
  [[ -n "${fmt}" ]] || continue

  name="$(safe_name "${src}")"
  log "INSPECT ${src} (fmt=${fmt})"
  if ! run_info_and_check "${src}" "${fmt}" "${name}"; then
    echo "${name}__inspect" >> "${SUMMARY_FAIL}"
  else
    echo "${name}__inspect" >> "${SUMMARY_OK}"
  fi

  for outfmt in raw qcow2 vdi; do
    test_conversion_case_wrapper "${src}" "${fmt}" "${outfmt}"
  done
done

run_feature_suite

log "================ SUMMARY ================"
OK_COUNT="$(wc -l < "${SUMMARY_OK}" | tr -d ' ')"
FAIL_COUNT="$(wc -l < "${SUMMARY_FAIL}" | tr -d ' ')"
log "OK_COUNT=${OK_COUNT}"
log "FAIL_COUNT=${FAIL_COUNT}"

if [[ -s "${SUMMARY_FAIL}" ]]; then
  log "Failed cases:"
  cat "${SUMMARY_FAIL}" >&2
  exit 1
fi

touch "${WORKDIR}/.success"
log "All tests passed"
log "Logs: ${LOG_DIR}"
log "Outputs: ${OUT_DIR}"
