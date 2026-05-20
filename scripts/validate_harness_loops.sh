#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
LOOPS_DIR="${ROOT_DIR}/harness/loops"
HOSTS_DIR="${ROOT_DIR}/harness/hosts"
BINDINGS_DIR="${ROOT_DIR}/harness/bindings"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

validate_loop() {
  local loop_dir="$1"
  local manifest="${loop_dir}/loop.json"
  local name

  if [[ ! -f "${manifest}" ]]; then
    echo "missing loop manifest: ${manifest}" >&2
    return 1
  fi

  jq . "${manifest}" >/dev/null
  name="$(jq -r '.name // empty' "${manifest}")"
  if [[ -z "${name}" ]]; then
    echo "loop manifest missing name: ${manifest}" >&2
    return 1
  fi
  if [[ "$(jq -r '.schema_version // 0' "${manifest}")" -lt 2 ]]; then
    echo "loop manifest schema_version must be 2 or higher: ${manifest}" >&2
    return 1
  fi
  for field in control_model entity_profiles surfaces; do
    if [[ "$(jq -r "has(\"${field}\")" "${manifest}")" != "true" ]]; then
      echo "loop manifest missing ${field}: ${manifest}" >&2
      return 1
    fi
  done
  for field in state intent reality reconcile; do
    if [[ "$(jq -r ".control_model | has(\"${field}\")" "${manifest}")" != "true" ]]; then
      echo "loop control_model missing ${field}: ${manifest}" >&2
      return 1
    fi
  done
  for field in projection observation; do
    if [[ "$(jq -r ".surfaces | has(\"${field}\")" "${manifest}")" != "true" ]]; then
      echo "loop surfaces missing ${field}: ${manifest}" >&2
      return 1
    fi
  done

  while IFS= read -r rel; do
    [[ -n "${rel}" ]] || continue
    if [[ ! -e "${loop_dir}/${rel}" ]]; then
      echo "missing ${name} asset: ${rel}" >&2
      return 1
    fi
  done < <(
    jq -r '
      .assets.guide,
      .assets.env,
      ((.assets.runtime_files // [])[]),
      (.assets.hooks[]),
      (.assets.skills[]),
      (.assets.subagents[])
    ' "${manifest}"
  )

  while IFS= read -r rel; do
    [[ -n "${rel}" ]] || continue
    if [[ ! -e "${loop_dir}/${rel}" ]]; then
      echo "missing ${name} host adapter path: ${rel}" >&2
      return 1
    fi
  done < <(jq -r '.host_adapters[]' "${manifest}")

  echo "ok ${name}"
}

validate_host() {
  local host_manifest="$1"
  local name

  jq . "${host_manifest}" >/dev/null
  name="$(jq -r '.name // empty' "${host_manifest}")"
  if [[ -z "${name}" ]]; then
    echo "host manifest missing name: ${host_manifest}" >&2
    return 1
  fi
  if [[ "$(jq -r '.schema_version // 0' "${host_manifest}")" -lt 2 ]]; then
    echo "host manifest schema_version must be 2 or higher: ${host_manifest}" >&2
    return 1
  fi
  for field in surfaces lifecycle_mapping; do
    if [[ "$(jq -r "has(\"${field}\")" "${host_manifest}")" != "true" ]]; then
      echo "host manifest missing ${field}: ${host_manifest}" >&2
      return 1
    fi
  done
  for field in projection observation; do
    if [[ "$(jq -r ".surfaces | has(\"${field}\")" "${host_manifest}")" != "true" ]]; then
      echo "host surfaces missing ${field}: ${host_manifest}" >&2
      return 1
    fi
  done

  echo "ok host ${name}"
}

validate_binding() {
  local binding_manifest="$1"
  local name host loop

  jq . "${binding_manifest}" >/dev/null
  name="$(jq -r '.name // empty' "${binding_manifest}")"
  host="$(jq -r '.host // empty' "${binding_manifest}")"
  loop="$(jq -r '.loop // empty' "${binding_manifest}")"
  if [[ -z "${name}" || -z "${host}" || -z "${loop}" ]]; then
    echo "binding manifest missing name, host, or loop: ${binding_manifest}" >&2
    return 1
  fi
  if [[ ! -f "${HOSTS_DIR}/${host}/host.json" ]]; then
    echo "binding references missing host: ${binding_manifest}" >&2
    return 1
  fi
  if [[ ! -f "${LOOPS_DIR}/${loop}/loop.json" ]]; then
    echo "binding references missing loop: ${binding_manifest}" >&2
    return 1
  fi
  for field in projection_path runtime_surface lifecycle_mapping reconcile; do
    if [[ "$(jq -r "has(\"${field}\")" "${binding_manifest}")" != "true" ]]; then
      echo "binding manifest missing ${field}: ${binding_manifest}" >&2
      return 1
    fi
  done

  echo "ok binding ${name}"
}

for loop_dir in "${LOOPS_DIR}"/*; do
  [[ -d "${loop_dir}" ]] || continue
  validate_loop "${loop_dir}"
done

for host_manifest in "${HOSTS_DIR}"/*/host.json; do
  [[ -f "${host_manifest}" ]] || continue
  validate_host "${host_manifest}"
done

for binding_manifest in "${BINDINGS_DIR}"/*.json; do
  [[ -f "${binding_manifest}" ]] || continue
  validate_binding "${binding_manifest}"
done
