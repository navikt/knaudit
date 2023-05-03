# Knaudit

> KNADA audit logger

Sender loggdata basert på Kubernetes og Airflow-jobber til Datavarehus.

Følgende data logges:

* Hostname
* IP
* Namespace
* DAG ID
* Run ID
* Task ID
* Hvem som startet DAG/job
* Git repo
* Commit SHA1
* DAG/Job startet
* Timestamp

## Eksempel output

```json
{
  "commit_sha1": [
    "d19dcf695f043c6eff6b0cc2478b58d45299ca97"
  ],
  "hostname": [
    "mycsvdag-starting-fc8dfe28afae414da33a5d2a57db85d1"
  ],
  "run_id": [
    "scheduled__2023-05-03T05:30:00+00:00"
  ],
  "timestamp": [
    "2023-05-03T05:35:11.000Z"
  ],
  "git_repo": [
    "github.com/navikt/test-team-dag"
  ],
  "ip": [
    "321.312.312.321"
  ],
  "namespace": [
    "team-test-ncnv"
  ],
  "task_id": [
    "starting"
  ],
  "git_branch": [
    "main"
  ],
  "dag_id": [
    "MyCSVDAG"
  ],
  "triggered_by": [
    "airflow"
  ]
}
```
