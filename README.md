# terra-disk-manager
cronjob to attach snapshot schedules to persistent disks created by k8s stateful sets

## Deployment TODOs

* ArgoCD Deployments
  * Chelsea: figure out how to deploy this independently from terra-cluster without manually creating the project / Argo app
  * New namespace?
    * terra-cluster-disk-manager
    * terra-disk-manager?
    * cluster-disk-manager
    * disk-manager <- winner! simplest, doesn't conflict with terra- namespaces, indicates internal / not part of Terra
  * New helm chart?
    * disk-manager!
* Terraform module
    * New terraform module to provision the gcp service account
    * Deployed as part of the terra-cluster Atlantis project
    * GCP SA permissions:
      * Needs to be able to list disks and update resource policy (not sure exact perms)
* Cloud Build
  * Need to build docker image and publish to dsp-artifact-registry
  * Mike already set up CloudBuild integration, needs to build the thingy / get configured.
* Helm chart
  * Create K8s service account with permission list PVCs and PVs in all namespaces (?)
    * App needs to be updated to use this SA... We shall test/experiment!
  * Secrets manager to pull in GCP cred
    * App needs to be updated to use the GCP SA.
  * Deploy the cronjob
    * Make sure we know if starts failing! Mike: Prometheus does this already! :awesome: :awesome: :awesome:

