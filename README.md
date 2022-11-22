# action-conftest

A GitHub Action for easily using [conftest](https://github.com/open-policy-agent/conftest) in your CI. It allows for pulling policies from another source and can surface the violations and warnings into the comments of the pull request. Additionally, the action can submit metrics for the results of the tests to a remote server for analysis of the rate of failures and warnings, which is useful when deploying new policies.

**NOTE:** This action only supports pull secrets for S3, GCS, and HTTP remotes. If you are pulling from an OCI registry, it is assumed that you have already authenticated using `docker login` in a previous step in the GitHub Actions `job` and you should not supply a `pull-secret` argument.

## Options

| Option          | Description                                                     | Default  | Required               |
|-----------------|-----------------------------------------------------------------|----------|------------------------|
| files           | Files and/or folders for Conftest to test (space delimited)     |          | yes                    |
| policy          | Where to find the policy folder or file                         | policy   | no                     |
| data            | Files or folder with supplemental test data                     |          | no                     |
| all-namespaces  | Whether to use all namespaces in testing                        | true     | no                     |
| combine         | Whether to combine input files                                  | false    | no                     |
| pull-url        | URL to pull policies from                                       |          | no                     |
| pull-secret     | Secret that allows the policies to be pulled                    |          | no                     |
| add-comment     | Whether or not to add a comment to the PR                       | true     | no                     |
| docs-url        | Documentation URL to link to in the PR comment                  |          | no                     |
| no-fail         | Always returns an exit code of 0 (no error)                     | false    | no                     |
| gh-token        | Token to authorize adding the PR comment                        |          | if add-comment is true |
| gh-comment-url  | URL of the comments for the PR                                  |          | if add-comment is true |
| metrics-url     | URL to POST the results to for metrics                          |          | no                     |
| metrics-source  | Unique ID for the source of the metrics (usually the repo name) |          | if metrics-url is set  |
| metrics-details | Whether to include the full test results in the metrics         | false    | no
| metrics-token   | Bearer token for submitting the metrics                         |          | no                     |
| policy-id-key   | Name of the key in the details object that stores the policy ID | policyID | if metrics-url is set  |

## Example Usage

### Using policies already in the repo

This is a basic example. It assumes the policies already exist in the repository in the default `policy/` directory.

```yaml
name: conftest
on: [pull_request]
jobs:
  conftest:
    runs-on: ubuntu-latest
    steps:
      - name: checkout
        uses: actions/checkout@v3
      - name: conftest
        uses: YubicoLabs/action-conftest@v3
        with:
          files: some_deployment.yaml another_resource.yaml
          gh-token: ${{ secrets.GITHUB_TOKEN }}
          gh-comment-url: ${{ github.event.pull_request.comments_url }}
```

### Pulling policies from a Google Cloud Storage bucket

This example shows pulling the policy directory from a GCS bucket. In this case, the `pull-secret` variable is the JSON key for a service account with read acccess to the bucket.

```yaml
name: conftest-with-pull
on: [pull_request]
jobs:
  conftest:
    runs-on: ubuntu-latest
    steps:
      - name: checkout
        uses: actions/checkout@v3
      - name: conftest
        uses: YubicoLabs/action-conftest@v3
        with:
          files: some_deployment.yaml another_resource.yaml
          pull-url: gcs::https://www.googleapis.com/storage/v1/bucket_name/policy
          pull-secret: ${{ secrets.POLICY_PULL_SECRET }}
          gh-token: ${{ secrets.GITHUB_TOKEN }}
          gh-comment-url: ${{ github.event.pull_request.comments_url }}
```

### Submitting metrics to a remote server

```yaml
name: conftest-push-metrics
on: [pull_request]
jobs:
  conftest:
    runs-on: ubuntu-latest
    steps:
      - name: checkout
        uses: actions/checkout@v3
      - name: conftest
        uses: YubicoLabs/action-conftest@v3
        with:
          files: some_deployment.yaml another_resource.yaml
          gh-token: ${{ secrets.GITHUB_TOKEN }}
          gh-comment-url: ${{ github.event.pull_request.comments_url }}
          metrics-url: https://your.com/metrics/endpoints/conftest
          metrics-source: your-repo-name
```
