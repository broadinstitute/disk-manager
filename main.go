package main

import (
	"github.com/broadinstitute/terra-disk-manager/client"
	"k8s.io/client-go/kubernetes"

	//    "context"

	"fmt"
	"log"
	"net/http"

	"google.golang.org/api/compute/v1"
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

	fmt.Println(disks)

	// GCP poc starts here
	gcp, err := compute.New(http.DefaultClient)
	if err != nil {
		panic(err.Error())
	}

	result, err := gcp.Disks.List("broad-dsde-dev", "us-central1-a").Do()
	if err != nil {
		panic(err.Error())
	}
	r2 := *result
	fmt.Printf("%d\n", len(r2.Items))
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

			// retrive associated persistent volume
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
