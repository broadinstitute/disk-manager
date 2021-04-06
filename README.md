# disk-manager

[![Go Report Card](https://goreportcard.com/badge/github.com/broadinstitute/disk-manager)](https://goreportcard.com/report/github.com/broadinstitute/disk-manager)

Disk-manager is a tool to manage google compute engine persistent disks dynamically provisioned by kubernetes stateful sets. It ensures that
the GCE persistent disks have a snapshot schedule attached so that the disks are backed up regularly. The snapshot scheudle that will be attached is
determined by an annotation on `persistentVolumeClaims` created as part of stateful kubernetes deployments.

## Usage

Disk-manager utilizes annotations on `persistentVolumeClaims` in order to look up look up the underlying GCE persistent disk.
This can be specified as part of the `volumeClaimTemplates` stanza of a `statefulSet`.

Example:

```
volumeClaimTemplates:
  - metadata:
      name: example-volume-claim
      annotations:
        bio.terra/snapshot-policy: SNAPSHOT_SCHEDULE_NAME
```

The annotation key that disk manager uses can be specified in the disk-manager config. For broadinstitute terra clusters,
the annotation key is: `bio.terra/snapshot-policy`. The snapshot schedule name must reference a pre-existing snapshot schedule in GCP.
Currently disk-manager only associates persistent disks with existing snapshot schedules. It will not create new snapshot schedules.

Once disk-manager is installed in a cluster and the appropriate annotation has been added to stateful deployments, disk manager will
automatically detect the compute engine disks for each stateful set and add the desired snapshot schedule with no other action needed.

With default settings the disk-manager cronjob will run everyday at 1 AM UTC.

## Installation

Disk-manager is intended to be deployed as a kubernetes cronjob. A helm chart to install disk-manager is available [here](https://github.com/broadinstitute/terra-helm/tree/master/charts/diskmanager). Details on configuring the helm chart are available at that link. The cronjob is intended to be deployed within the same cluster as the stateful sets it manages.

When deployed as a cronjob via helm, disk-manager will use in cluster authentication provided by [kubernetes/client-go](https://github.com/kubernetes/client-go). There are no additional steps required to configure this. The helm chart will create a `clusterRole` for diskmanager with the appropriate permissions.

Disk-manager also requires a GCP service account with `roles/compute.storageAdmin` in order to attach snapshot schedules to the gce disks associated with kubernetes `persistentVolumes`
Google [Application Default Credentials(ADC)](https://cloud.google.com/docs/authentication/production#automatically) is used to authenticate to GCP. The cronjob expects service account
credentials to be mounted to the pod using this pattern.

### Running Locally

While the intended use for disk-manager is to run as a kubernetes cronjob it is also possible to run the tool locally against a remote cluster.
A public docker image is available at `us-central1-docker.pkg.dev/dsp-artifact-registry/disk-manager/disk-manager:main`

When running the docker image locally the `-local` runtime flag must be used. This tells disk manager to connect to a remote cluster
using your local `.kube/config` otherwise in cluster authentication will be used. Your local `.kubconfig` and a GCP credential must be mounted to the
container when running locally.

Unit tests can be run with `go test`:

```
    # Run tests w/ coverage stats
    go test -coverprofile=coverage.out

    # View line-by-line coverage report in browser
    go tool cover -html=coverage.out
```

### Runtime flags

```
Usage of disk-manager:
  -config-file string
    	path to yaml file with disk-manager config (default "/etc/disk-manger/config.yaml")
  -kubeconfig string
    	(optional) absolute path to kubectl config (default "~/.kube/config")
  -local
    	use this flag when running locally (outside of cluster to use local kube config
```

### Configuration
Disk manager does require a small number of configuration values. When deploying via helm these are managed by a `configMap` and
specified using helm values.
 
When running locally a config file is expected to be at `/etc/disk-manager/config.yaml`. The path can be specified with the `-config-file` flag.
The config file expects the following format

```
targetAnnotation: terra.bio/snapshot-policy # The annotation key disk-manager uses to determine which persistent volume claims to operate on
googleProject: GCP_PROJECT_ID
region: GCP_REGION
zone: GCP_ZONE
```
