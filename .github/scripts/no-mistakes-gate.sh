#!/usr/bin/env bash
#
# Decides whether a pull request satisfies the "PR must be raised via no-mistakes"
# required check.
#
# A PR passes when any of these hold:
#   * its body carries the no-mistakes pipeline signature AND a parseable v1
#     pipeline step attestation whose head_sha equals the current PR head and
#     in which review, test, and document are status=completed, or
#   * it was opened by github-actions[bot] or dependabot[bot], or
#   * it is structurally a release-please release PR (see below).
#
# The release-please exemption is deliberately STRUCTURAL, never author identity.
# Today release-please opens treehouse's release PRs as github-actions[bot], so
# the bot exemption already covers them, but the structural test keeps them
# covered if release-please is ever switched to a PAT and starts arriving as the
# human `kunchenguid`, who also opens ordinary human PRs. Exempting that login
# would exempt every human PR too.
#
# Every exemption lives here rather than in a job-level `if:` so the whole gate
# has one executable surface that tests can drive directly.
#
# Inputs (environment):
#   PR_BODY, PR_AUTHOR, PR_NUMBER, PR_HEAD_REF, PR_HEAD_REPO, PR_BASE_REPO,
#   PR_HEAD_SHA (github.event.pull_request.head.sha; required on the
#   attestation path so a later push cannot pass on a stale comment)
# Exit status: 0 = pass, 1 = fail.
set -eu

NO_MISTAKES_MARKER='Updates from [git push no-mistakes](https://github.com/kunchenguid/no-mistakes)'
ATTESTATION_PREFIX='<!-- no-mistakes-pipeline-attestation:v1'
ATTESTATION_VERSION_FLOOR='1.46.0'
ATTESTATION_VERSION_URL='https://github.com/kunchenguid/no-mistakes/pull/670'
RELEASE_PLEASE_MARKER='This PR was generated with [Release Please]'
RELEASE_PLEASE_BRANCH_PREFIX='release-please--'
RELEASE_PLEASE_LEGACY_BRANCH_PREFIX='release-please/'

pr_body="${PR_BODY:-}"
pr_author="${PR_AUTHOR:-unknown}"
pr_number="${PR_NUMBER:-unknown}"
pr_head_ref="${PR_HEAD_REF:-}"
pr_head_repo="${PR_HEAD_REPO:-}"
pr_base_repo="${PR_BASE_REPO:-}"
pr_head_sha="${PR_HEAD_SHA:-}"
pr_head_sha=${pr_head_sha//$'\r'/}

body_contains() {
    printf '%s' "$pr_body" | grep -qF -- "$1"
}

is_exempt_bot() {
    case "$pr_author" in
        'github-actions[bot]' | 'dependabot[bot]') return 0 ;;
        *) return 1 ;;
    esac
}

# Condition 1: a branch under release-please's reserved branch prefix. The
# legacy `release-please/` prefix is accepted for older release-please setups.
is_release_please_branch() {
    case "$pr_head_ref" in
        "$RELEASE_PLEASE_BRANCH_PREFIX"* | "$RELEASE_PLEASE_LEGACY_BRANCH_PREFIX"*) return 0 ;;
        *) return 1 ;;
    esac
}

# Condition 2: same-repo head. A fork can copy the branch name and the body but
# cannot make its head repository be this repository.
is_same_repo_head() {
    [ -n "$pr_head_repo" ] && [ -n "$pr_base_repo" ] && [ "$pr_head_repo" = "$pr_base_repo" ]
}

# Condition 3: release-please's generated body footer.
has_release_please_footer() {
    body_contains "$RELEASE_PLEASE_MARKER"
}

json_python() {
    local cand
    for cand in python3 python; do
        if command -v "$cand" >/dev/null 2>&1 && "$cand" -c 'import json,re,sys' >/dev/null 2>&1; then
            command -v "$cand"
            return 0
        fi
    done
    return 1
}

fail_missing_or_unparseable_attestation() {
    {
        echo "::error::This PR has the no-mistakes signature but no parseable pipeline step attestation."
        echo
        echo "This repository requires no-mistakes >= ${ATTESTATION_VERSION_FLOOR} (${ATTESTATION_VERSION_URL})."
        echo "Older no-mistakes that only writes the signature line is not enough."
        echo "Submit via 'git push no-mistakes' with a current no-mistakes so the PR body includes:"
        echo
        echo "    ${ATTESTATION_PREFIX} {\"head_sha\":\"...\",\"steps\":[...]} -->"
        echo
        echo "PR author: ${pr_author}"
    } >&2
    exit 1
}

fail_incomplete_attestation() {
    {
        echo "::error::This PR's no-mistakes pipeline attestation does not show completed required steps."
        echo
        echo "This repository requires review, test, and document to each have status=completed."
        echo "Quota skips and agent skips are not compliant."
        echo
        printf '%s\n' "$@"
        echo
        echo "Re-run those steps to completion with 'git push no-mistakes'."
        echo
        echo "PR author: ${pr_author}"
    } >&2
    exit 1
}

display_sha() {
    if [ -n "$1" ]; then
        printf '%s' "$1"
    else
        printf '%s' '(missing)'
    fi
}

fail_attestation_head_sha() {
    local attested_sha="$1"
    {
        echo "::error::This PR's no-mistakes pipeline attestation is not bound to the current pull request head."
        echo
        echo "The attestation head_sha must equal the current pull request head SHA so a later push cannot pass on a stale attestation."
        echo "  attestation head_sha: $(display_sha "$attested_sha")"
        echo "  pull request head:    $(display_sha "$pr_head_sha")"
        echo
        echo "Re-run 'git push no-mistakes' on the current head so the PR body attestation is rewritten for this commit."
        echo
        echo "PR author: ${pr_author}"
    } >&2
    exit 1
}

# Reads PR_BODY and prints one of:
#   MISSING
#   UNPARSEABLE
#   PARSED
#   head_sha=<value>   (empty when the field is missing or not a string)
#   review=<status>
#   test=<status>
#   document=<status>
# Status is "missing" when the step is absent or not a string.
parse_pipeline_attestation() {
    local py
    py=$(json_python) || {
        echo UNPARSEABLE
        return 0
    }
    "$py" - <<'PY'
import json, os, re, sys

body = os.environ.get("PR_BODY", "")
match = re.search(
    r"<!--\s*no-mistakes-pipeline-attestation:v1\s+(.*?)\s*-->",
    body,
    flags=re.DOTALL,
)
if match is None:
    sys.stdout.write("MISSING\n")
    sys.exit(0)

raw = match.group(1).strip()
try:
    payload = json.loads(raw)
except json.JSONDecodeError:
    sys.stdout.write("UNPARSEABLE\n")
    sys.exit(0)

if not isinstance(payload, dict):
    sys.stdout.write("UNPARSEABLE\n")
    sys.exit(0)

raw_head_sha = payload.get("head_sha")
if isinstance(raw_head_sha, str):
    head_sha = raw_head_sha.strip()
else:
    head_sha = ""
steps = payload.get("steps")
if not isinstance(steps, list):
    sys.stdout.write("UNPARSEABLE\n")
    sys.exit(0)

statuses = {}
for item in steps:
    if not isinstance(item, dict):
        sys.stdout.write("UNPARSEABLE\n")
        sys.exit(0)
    name = item.get("step")
    status = item.get("status")
    if not isinstance(name, str) or not name:
        sys.stdout.write("UNPARSEABLE\n")
        sys.exit(0)
    if name in statuses:
        continue
    if isinstance(status, str) and status:
        statuses[name] = status
    else:
        statuses[name] = "missing"

sys.stdout.write("PARSED\n")
sys.stdout.write("head_sha=%s\n" % head_sha)
for name in ("review", "test", "document"):
    sys.stdout.write("%s=%s\n" % (name, statuses.get(name, "missing")))
PY
}

require_pipeline_attestation() {
    local parsed kind pair name status rest attested_sha=""
    local -a incomplete=()

    parsed=$(parse_pipeline_attestation || echo UNPARSEABLE)
    parsed=$(printf '%s\n' "$parsed" | tr -d '\r')
    kind=${parsed%%$'\n'*}
    case "$kind" in
        PARSED) ;;
        *) fail_missing_or_unparseable_attestation ;;
    esac

    rest=${parsed#*$'\n'}
    while IFS= read -r pair || [ -n "$pair" ]; do
        [ -n "$pair" ] || continue
        name=${pair%%=*}
        status=${pair#*=}
        if [ "$name" = head_sha ]; then
            attested_sha=$status
            continue
        fi
        if [ "$status" != completed ]; then
            incomplete+=("  ${name}: ${status}")
        fi
    done <<EOF
$rest
EOF
    if [ -z "$attested_sha" ] || [ -z "$pr_head_sha" ] || [ "$attested_sha" != "$pr_head_sha" ]; then
        fail_attestation_head_sha "$attested_sha"
    fi
    if [ "${#incomplete[@]}" -ne 0 ]; then
        fail_incomplete_attestation "${incomplete[@]}"
    fi
}

if is_exempt_bot; then
    echo "PR #${pr_number} was opened by ${pr_author}; exempt from the no-mistakes signature."
    exit 0
fi

if is_release_please_branch && is_same_repo_head && has_release_please_footer; then
    echo "PR #${pr_number} is a release-please release PR (same-repo branch '${pr_head_ref}' with the Release Please footer); exempt from the no-mistakes signature."
    exit 0
fi

if body_contains "$NO_MISTAKES_MARKER"; then
    require_pipeline_attestation
    echo "Found no-mistakes signature and completed review/test/document attestation bound to head ${pr_head_sha} in PR #${pr_number} body."
    exit 0
fi

{
    echo "::error::This PR was not raised through no-mistakes."
    echo
    echo "Contributions to this repository must be submitted via 'git push no-mistakes'."
    echo "That pipeline runs the required review/test/lint/CI steps and writes a"
    echo "deterministic '## Pipeline' section into the PR body containing:"
    echo
    echo "    $NO_MISTAKES_MARKER"
    echo
    echo "The only other way to pass is release-please's own release PR, which must"
    echo "satisfy all three structural conditions: a '${RELEASE_PLEASE_BRANCH_PREFIX}'"
    echo "(or legacy '${RELEASE_PLEASE_LEGACY_BRANCH_PREFIX}') head branch, a"
    echo "same-repository (non-fork) head, and the Release Please body footer."
    echo
    echo "PR author: ${pr_author}"
    echo "Head branch: ${pr_head_ref:-unknown}"
    echo "Head repository: ${pr_head_repo:-unknown} (base ${pr_base_repo:-unknown})"
} >&2
exit 1
