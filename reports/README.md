# Verification evidence

This directory stores only evidence produced by an actual run. Do not replace commands, screenshots, reviews, deployment URLs, user feedback, or measurements with generated examples.

## Automated gates

Run from the repository root:

```bash
make check
make check-security
```

Save the complete terminal output under `reports/automated/` with the commit SHA and UTC timestamp in the filename.

## Performance

After `make up`, run:

```bash
docker run --rm -i grafana/k6:0.54.0 run - \
  -e BASE_URL=http://host.docker.internal:18088 \
  -e VUS=10 -e DURATION=30s \
  < tests/load/core-paths.js
```

Copy the unedited k6 output into `reports/performance/`. Record machine, network, model mode, VUs, duration, request count, failure rate, and latency percentiles. Do not claim a threshold until this run has completed.

## Human and external evidence

The following must be completed by group members:

- Pull request links and a non-author review for each member's module.
- A real red/green quality-gate run with the failing cause and fixing commit.
- A public deployment URL tested from a device outside the deployment network.
- Three real user feedback records, with consent and no unnecessary personal data.
- Screenshots of the deployed core paths and failure paths.

Use `reports/human-evidence/template.md`; leave an item explicitly pending until authentic evidence exists.
