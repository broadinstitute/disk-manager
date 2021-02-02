package main

import (
	"github.com/broadinstitute/terra-disk-manager/client"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"k8s.io/client-go/kubernetes"

	//    "context"

	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/openstack"
)

func main() {
	// create the k8s client (technically a "clientset")
	k8s, err := client.Build()
	if err != nil {
		log.Fatalf("Error building k8s client: %v, exiting\n", err)
	}

	// TODO - pagination needed?
	disks, err := getDisks(k8s)
	if err != nil {
		log.Fatalf("Error retrieving persistent disks: %v\n", err)
	}

	// GCP poc starts here
	ctx := context.Background()

	gcpClient, err := google.DefaultClient(ctx, compute.CloudPlatformScope)
	if err != nil {
		log.Fatalf("Error creating GCP client: %v\n", err)
	}

	c, err := compute.New(gcpClient)
	if err != nil {
		log.Fatalf("Error creating compute API client: %v\n", err)
	}

	// hardcoded params for compute api query
	project := "broad-dsde-dev"
	zone := "us-central1-a"
	policyName := "terra-snapshot-policy"

	for _, disk := range disks {
		addPolicyRequest := &compute.DisksAddResourcePoliciesRequest{
			ResourcePolicies: []string{policyName},
		}
		resp, err := c.Disks.AddResourcePolicies(project, zone, disk, addPolicyRequest).Context(ctx).Do()
		if err != nil {
			log.Printf("Error getting disk: %s, %v\n", disk, err)
		}
		fmt.Printf("%#v\n", resp)
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
