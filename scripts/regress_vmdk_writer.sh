#!/usr/bin/env bash
set -euo pipefail

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

ROOT=""
BIN=""
WORKDIR="/var/tmp/imgconv_regress_vmdk_writer"
THREADS="${THREADS:-4}"
CHUNK_MIB="${CHUNK_MIB:-4}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root) ROOT="$2"; shift 2 ;;
    --bin) BIN="$2"; shift 2 ;;
    --workdir) WORKDIR="$2"; shift 2 ;;
    --threads) THREADS="$2"; shift 2 ;;
    --chunk-mib) CHUNK_MIB="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

[[ -n "${ROOT}" ]] || { echo "--root is required" >&2; exit 1; }
[[ -d "${ROOT}" ]] || { echo "root not found: ${ROOT}" >&2; exit 1; }
[[ -n "${BIN}" ]] || { echo "--bin is required" >&2; exit 1; }
[[ -x "${BIN}" ]] || { echo "binary not executable: ${BIN}" >&2; exit 1; }

need qemu-img
need cmp
need sha256sum

mkdir -p "${WORKDIR}"
LOG="${WORKDIR}/regress_vmdk_writer.log"
: > "${LOG}"

QCOW2_SRC="$(find "${ROOT}" -type f -name 'Rocky-9-GenericCloud.latest.x86_64.qcow2' | head -n1)"
[[ -n "${QCOW2_SRC}" ]] || { echo "Rocky qcow2 not found" >&2; exit 1; }

VDI_SRC="$(find "${ROOT}/vdi_calculate" -type f -name '*.vdi' | sort | head -n1)"
[[ -n "${VDI_SRC}" ]] || { echo "no vdi found in vdi_calculate" >&2; exit 1; }

VMDK_SRC="$(find "${ROOT}/vmdk_calculate" -type f -name '*.vmdk' | sort | head -n1)"
[[ -n "${VMDK_SRC}" ]] || { echo "no vmdk found in vmdk_calculate" >&2; exit 1; }

RAW_SEED="${WORKDIR}/seed.raw"
python3 - <<PY
from pathlib import Path
p = Path(r"${RAW_SEED}")
data = bytearray(8*1024*1024)
data[0:16] = b"seed-vmdk-000001"
data[2*1024*1024:2*1024*1024+16] = b"seed-vmdk-000002"
data[-16:] = b"seed-vmdk-tail!!"
p.write_bytes(data)
PY

echo "== create empty vmdk" | tee -a "${LOG}"
"${BIN}" create -f vmdk -o "${WORKDIR}/empty.vmdk" --size 64M | tee -a "${LOG}"
"${BIN}" info -i "${WORKDIR}/empty.vmdk" --input-format vmdk | tee -a "${LOG}"
qemu-img info "${WORKDIR}/empty.vmdk" | tee -a "${LOG}"

run_case() {
  local src="$1"
  local infmt="$2"
  local name="$3"

  local out="${WORKDIR}/${name}.vmdk"
  local src_raw="${WORKDIR}/${name}_src.raw"
  local out_raw="${WORKDIR}/${name}_out.raw"

  echo "== convert ${name} (${infmt} -> vmdk)" | tee -a "${LOG}"
  "${BIN}" convert \
    -i "${src}" \
    -o "${out}" \
    --input-format "${infmt}" \
    --format vmdk \
    --threads "${THREADS}" \
    --chunk-mib "${CHUNK_MIB}" \
    --sparse \
    --verify full | tee -a "${LOG}"

  "${BIN}" info -i "${out}" --input-format vmdk | tee -a "${LOG}"
  "${BIN}" check -i "${out}" --input-format vmdk | tee -a "${LOG}"
  qemu-img info "${out}" | tee -a "${LOG}"

  case "${infmt}" in
    raw) cp --sparse=always "${src}" "${src_raw}" ;;
    qcow2|vdi|vmdk) qemu-img convert -f "${infmt}" -O raw "${src}" "${src_raw}" ;;
    *) echo "unsupported input fmt ${infmt}" >&2; exit 1 ;;
  esac

  qemu-img convert -f vmdk -O raw "${out}" "${out_raw}"
  cmp "${src_raw}" "${out_raw}"
  sha256sum "${src_raw}" "${out_raw}" | tee -a "${LOG}"
}

run_case "${RAW_SEED}" raw raw_seed
run_case "${QCOW2_SRC}" qcow2 rocky_qcow2
run_case "${VDI_SRC}" vdi calculate_vdi
run_case "${VMDK_SRC}" vmdk calculate_vmdk

echo "OK: regress_vmdk_writer passed" | tee -a "${LOG}"
echo "Workdir: ${WORKDIR}" | tee -a "${LOG}"