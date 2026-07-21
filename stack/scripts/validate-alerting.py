#!/usr/bin/env python3
"""Validate Grafana alerting provisioning before it reaches a deploy.

Grafana treats an invalid alerting provisioning file as a FATAL startup error,
not a skippable warning: the provisioning module fails, every module depending
on it fails with it, and the container crash-loops. On 2026-07-21 that took the
whole observability stack down twice in a row - first on an empty contact-point
url, then on a rule UID of 47 characters - each discovered only by deploying.

This script checks the constraints that produce that failure mode, so they fail
in CI on a pull request instead of in production.

Usage: python3 stack/scripts/validate-alerting.py [alerting_dir] [datasources_file]
Exits non-zero and prints every problem found.
"""

import glob
import os
import re
import sys

try:
    import yaml
except ImportError:
    sys.exit("PyYAML is required: pip install pyyaml")

# Grafana's documented limits.
MAX_UID_LEN = 40
MAX_TITLE_LEN = 190
VALID_NO_DATA_STATE = {"NoData", "Alerting", "OK", "KeepLast"}
VALID_EXEC_ERR_STATE = {"OK", "Alerting", "Error", "KeepLast"}
# Grafana accepts a Go duration here; these are the units that make sense.
DURATION_RE = re.compile(r"^\d+[smhd]$")
# Expression queries use a sentinel datasource rather than a provisioned one.
EXPRESSION_DATASOURCE_UIDS = {"__expr__", "-100"}


def load_provisioned_datasource_uids(path):
    if not os.path.exists(path):
        return None  # caller decides whether that is fatal
    with open(path, encoding="utf-8") as handle:
        doc = yaml.safe_load(handle) or {}
    return {ds.get("uid") for ds in doc.get("datasources", []) if ds.get("uid")}


def validate(alerting_dir, datasources_file):
    problems = []
    datasource_uids = load_provisioned_datasource_uids(datasources_file)
    if datasource_uids is None:
        problems.append(f"datasources file not found: {datasources_file}")
        datasource_uids = set()

    seen_uids = {}
    seen_titles = {}
    rule_count = 0

    for path in sorted(glob.glob(os.path.join(alerting_dir, "*.yaml"))):
        with open(path, encoding="utf-8") as handle:
            doc = yaml.safe_load(handle) or {}
        name = os.path.basename(path)

        for contact_point in doc.get("contactPoints", []) or []:
            for receiver in contact_point.get("receivers", []) or []:
                uid = receiver.get("uid", "")
                if len(uid) > MAX_UID_LEN:
                    problems.append(
                        f"{name}: contact point uid {uid!r} is {len(uid)} chars (max {MAX_UID_LEN})"
                    )
                settings = receiver.get("settings", {}) or {}
                # An empty url provisions as invalid and is fatal. A ${VAR}
                # placeholder is fine - it is expanded from Grafana's process
                # environment at load time - but the variable must actually be
                # passed into the container, which compose-side .env is not.
                if receiver.get("type") == "webhook" and not settings.get("url"):
                    problems.append(f"{name}: webhook receiver {uid!r} has no url")

        for group in doc.get("groups", []) or []:
            folder = group.get("folder")
            group_name = group.get("name")
            interval = group.get("interval")
            if interval and not DURATION_RE.match(str(interval)):
                problems.append(f"{name}: group {group_name!r} bad interval {interval!r}")

            for rule in group.get("rules", []) or []:
                rule_count += 1
                uid = rule.get("uid", "")
                title = rule.get("title", "")

                if not uid:
                    problems.append(f"{name}: rule {title!r} has no uid")
                elif len(uid) > MAX_UID_LEN:
                    problems.append(
                        f"{name}: rule uid {uid!r} is {len(uid)} chars (max {MAX_UID_LEN})"
                    )
                if uid in seen_uids:
                    problems.append(f"{name}: duplicate rule uid {uid!r} (also in {seen_uids[uid]})")
                seen_uids[uid] = name

                if len(title) > MAX_TITLE_LEN:
                    problems.append(
                        f"{name}: rule title for {uid!r} is {len(title)} chars (max {MAX_TITLE_LEN})"
                    )
                title_key = (folder, group_name, title)
                if title_key in seen_titles:
                    problems.append(f"{name}: duplicate rule title {title!r} in {folder}/{group_name}")
                seen_titles[title_key] = name

                if rule.get("noDataState") not in VALID_NO_DATA_STATE:
                    problems.append(
                        f"{name}: rule {uid!r} noDataState {rule.get('noDataState')!r} "
                        f"not in {sorted(VALID_NO_DATA_STATE)}"
                    )
                if rule.get("execErrState") not in VALID_EXEC_ERR_STATE:
                    problems.append(
                        f"{name}: rule {uid!r} execErrState {rule.get('execErrState')!r} "
                        f"not in {sorted(VALID_EXEC_ERR_STATE)}"
                    )

                for_value = rule.get("for")
                if for_value and not DURATION_RE.match(str(for_value)):
                    problems.append(f"{name}: rule {uid!r} bad for {for_value!r}")

                queries = rule.get("data", []) or []
                ref_ids = [q.get("refId") for q in queries]
                condition = rule.get("condition")
                if condition not in ref_ids:
                    problems.append(
                        f"{name}: rule {uid!r} condition {condition!r} not among refIds {ref_ids}"
                    )
                for query in queries:
                    ds_uid = query.get("datasourceUid")
                    if ds_uid and ds_uid not in EXPRESSION_DATASOURCE_UIDS and ds_uid not in datasource_uids:
                        problems.append(
                            f"{name}: rule {uid!r} refId {query.get('refId')!r} references "
                            f"unprovisioned datasourceUid {ds_uid!r}"
                        )

    return problems, rule_count


def main():
    alerting_dir = sys.argv[1] if len(sys.argv) > 1 else "stack/grafana/provisioning/alerting"
    datasources_file = (
        sys.argv[2] if len(sys.argv) > 2 else "stack/grafana/provisioning/datasources/datasources.yaml"
    )

    problems, rule_count = validate(alerting_dir, datasources_file)

    if problems:
        print(f"Grafana alerting provisioning INVALID - {len(problems)} problem(s):\n")
        for problem in problems:
            print(f"  - {problem}")
        print("\nGrafana fails fatally on these; fix before deploying.")
        return 1

    print(f"Grafana alerting provisioning OK ({rule_count} rules checked).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
