#!/usr/bin/env bash
# Scans the wwwee-server image against DISA's GPOS SRG XCCDF profile with
# OpenSCAP and fails unless every "fail" result is a documented, accepted
# finding (see scripts/openscap/accepted-findings.txt).
#
# There is no DISA-published container-specific STIG; per the DoD DevSecOps
# Enterprise Container Hardening Process Guide, the GPOS SRG is used to
# assess image-level controls in its absence.
#
# NOTE: the SCAP content pulled below is a community-maintained (Chainguard)
# GPOS-aligned datastream, not an artifact hosted by DISA itself. Treat this
# script as a fast local/CI compliance signal, not a substitute for the
# authoritative DISA STIG/SRG artifacts (public.cyber.mil/stigs) required in
# a formal ATO/FedRAMP assessment.
#
# Requires: Docker, and network access on first run to fetch the SCAP
# content (cached under .cache/openscap/ afterwards).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE_TAG="${1:-wwwee-server:openscap}"
OUT_DIR="$REPO_ROOT/openscap-out"
CACHE_DIR="$REPO_ROOT/.cache/openscap"
SCAP_URL="https://raw.githubusercontent.com/chainguard-dev/stigs/main/gpos/xml/scap/ssg/content/ssg-chainguard-gpos-ds.xml"
SCAP_FILE="$CACHE_DIR/ssg-chainguard-gpos-ds.xml"
PROFILE="xccdf_basic_profile_.check"
SCANNER_IMAGE="docker.io/chainguard/openscap:latest-dev"
ACCEPTED_FINDINGS="$REPO_ROOT/scripts/openscap/accepted-findings.txt"
CHECKER="$REPO_ROOT/scripts/openscap/check-results.py"

mkdir -p "$OUT_DIR" "$CACHE_DIR"

if [[ ! -s "$SCAP_FILE" ]]; then
  echo "Fetching GPOS SRG SCAP content..."
  curl -fsSLo "$SCAP_FILE" "$SCAP_URL"
fi

echo "Building $IMAGE_TAG..."
docker build -t "$IMAGE_TAG" "$REPO_ROOT"

echo "Scanning $IMAGE_TAG against profile $PROFILE..."
set +e
docker run --rm -u 0:0 --pid=host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$OUT_DIR":/out \
  -v "$SCAP_FILE":/scap/gpos-ds.xml:ro \
  --entrypoint sh \
  "$SCANNER_IMAGE" -c "
    oscap-docker image '$IMAGE_TAG' xccdf eval \
      --profile '$PROFILE' \
      --report /out/report.html \
      --results /out/results.xml \
      /scap/gpos-ds.xml
  "
scan_status=$?
set -e

# oscap exit codes: 0 = all pass, 1 = evaluation error, 2 = at least one
# rule failed (expected here; the finding-level gate runs next).
if [[ $scan_status -eq 1 ]]; then
  echo "openscap-scan: oscap-docker reported an evaluation error (exit 1)" >&2
  exit 1
fi

echo
echo "Report:  $OUT_DIR/report.html"
echo "Results: $OUT_DIR/results.xml"
echo

docker run --rm \
  -v "$OUT_DIR":/out \
  -v "$REPO_ROOT/scripts/openscap":/checker:ro \
  --entrypoint python3 \
  "$SCANNER_IMAGE" /checker/check-results.py /out/results.xml /checker/accepted-findings.txt
