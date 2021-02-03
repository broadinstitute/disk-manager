package main

import (
	"fmt"
	"log"

	"github.com/broadinstitute/disk-manager/client"
	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func main() {
	// create the k8s client (technically a "clientset")
	clients, err := client.Build()
	if err != nil {
		log.Fatalf("Error building clients: %v, exiting\n", err)
	}
	k8s := clients.GetK8s()

	// TODO - pagination needed?
	disks, err := getDisks(k8s)
	if err != nil {
		log.Fatalf("Error retrieving persistent disks: %v\n", err)
	}

	// GCP poc starts here

	ctx := context.Background()

	gcp := clients.GetGCP()

	if err := addPolicy(ctx, gcp, disks); err != nil {
		log.Fatalf("Error adding snapshot policy to disks: %v\n", err)
	}

}
func getDisks(k8s *kubernetes.Clientset) ([]string, error) {
	var disks []string

	// get persistent volume claims
	pvcs, err := k8s.CoreV1().PersistentVolumeClaims("").List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("Error retrieving persistent volume claims: %v", err)
	}

	for _, pvc := range pvcs.Items {
		if policy, ok := pvc.Annotations["bio.terra/snapshot-policy"]; ok {
			log.Printf(
				"pvc name: %s\nsnapshot policy: %s\nvolume name: %s\n",
				pvc.Name,
				policy,
				pvc.Spec.VolumeName,
			)

			// retrive associated persistent volume for each claim
			pv, err := k8s.CoreV1().PersistentVolumes().Get(pvc.Spec.VolumeName, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("Error retrieving persistent volume: %s, %v", pvc.Spec.VolumeName, err)
			}
			diskName := pv.Spec.GCEPersistentDisk.PDName
			log.Printf("GCP disk name: %s\n\n", diskName)
			disks = append(disks, diskName)
		}
	}
	return disks, nil
}

func addPolicy(ctx context.Context, gcp *compute.Service, disks []string) error {
	// hardcoded params for compute api query
	project := "broad-dsde-dev"
	zone := "us-central1-a"
	region := "us-central1"
	policyName := "terra-snapshot-policy"

	policy, err := gcp.ResourcePolicies.Get(project, region, policyName).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("Error retrieving snapshot polict: %v", err)
	}
	policyURL := policy.SelfLink

	for _, disk := range disks {
		addPolicyRequest := &compute.DisksAddResourcePoliciesRequest{
			ResourcePolicies: []string{policyURL},
		}
		_, err := gcp.Disks.AddResourcePolicies(project, zone, disk, addPolicyRequest).Context(ctx).Do()
		if err != nil {
			log.Printf("Error adding snapshot policy to disk %s: %v\n", disk, err)
		}
		log.Printf("Added snapshot policy: %s to disk: %s", policyName, disk)
	}
	return nil
}
