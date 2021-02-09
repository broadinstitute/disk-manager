package main

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/broadinstitute/disk-manager/client"
	"github.com/broadinstitute/disk-manager/config"
	"github.com/broadinstitute/disk-manager/logs"
	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/util/homedir"
)

func main() {
	a := parseArgs()
	config, err := config.Read(*a.configFile)
	if err != nil {
		logs.Error.Fatalf("Error building config: %v", err)
	}

	logs.Info.Printf("Building clients...")
	clients, err := client.Build(a.local, a.kubeconfig)
	if err != nil {
		logs.Error.Fatalf("Error building clients: %v, exiting\n", err)
	}
	k8s := clients.GetK8s()

	// TODO - pagination needed?
	logs.Info.Println("Searching for persistent disks...")
	disks, err := getDisks(k8s, config)
	if err != nil {
		logs.Error.Fatalf("Error retrieving persistent disks: %v\n", err)
	}

	ctx := context.Background()

	gcp := clients.GetGCP()

	logs.Info.Println("Adding snapshot policy to disks...")
	if err := addPolicy(ctx, gcp, disks, config); err != nil {
		logs.Error.Fatalf("Error adding snapshot policy to disks: %v\n", err)
	}
	logs.Info.Println("Finished updating disks, exiting...")
}
func getDisks(k8s *kubernetes.Clientset, config *config.Config) ([]string, error) {
	var disks []string

	// get persistent volume claims
	pvcs, err := k8s.CoreV1().PersistentVolumeClaims("").List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("Error retrieving persistent volume claims: %v", err)
	}
	for _, pvc := range pvcs.Items {
		// Todo: actually assign the policy specified in annotation
		if _, ok := pvc.Annotations[config.TargetAnnotation]; ok {
			// retrive associated persistent volume for each claim
			pv, err := k8s.CoreV1().PersistentVolumes().Get(pvc.Spec.VolumeName, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("Error retrieving persistent volume: %s, %v", pvc.Spec.VolumeName, err)
			}
			diskName := pv.Spec.GCEPersistentDisk.PDName
			logs.Info.Printf("found PersistentVolume: %q with disk: %q", pvc.GetName(), diskName)
			disks = append(disks, diskName)
		}
	}
	return disks, nil
}

func addPolicy(ctx context.Context, gcp *compute.Service, disks []string, config *config.Config) error {
	// hardcoded params for compute api query

	policyName := "terra-snapshot-policy"

	policy, err := gcp.ResourcePolicies.Get(config.GoogleProject, config.Region, policyName).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("Error retrieving snapshot polict: %v", err)
	}
	policyURL := policy.SelfLink

	for _, disk := range disks {
		// check to make sure disk doesn't already have a snapshot policy
		hasPolicy, err := hasSnapshotPolicy(ctx, gcp, disk, config.GoogleProject, config.Zone)
		if err != nil {
			logs.Warn.Printf("unable to determine if disk %s has pre-existing resource policy, attempting to add %s", disk, policyName)
		}
		if hasPolicy {
			logs.Info.Printf("disk: %s already has snapshot policy... skipping", disk)
			continue
		}
		addPolicyRequest := &compute.DisksAddResourcePoliciesRequest{
			ResourcePolicies: []string{policyURL},
		}
		_, err = gcp.Disks.AddResourcePolicies(config.GoogleProject, config.Zone, disk, addPolicyRequest).Context(ctx).Do()
		if err != nil {
			logs.Error.Printf("Error adding snapshot policy to disk %s: %v\n", disk, err)
		}
		logs.Info.Printf("Added snapshot policy: %s to disk: %s", policyName, disk)
	}
	return nil
}

func hasSnapshotPolicy(ctx context.Context, gcp *compute.Service, disk, project, zone string) (bool, error) {
	resp, err := gcp.Disks.Get(project, zone, disk).Context(ctx).Do()
	if err != nil {
		return false, fmt.Errorf("Error determining if disk %s has pre-existing resource policy: %v", disk, err)
	}
	if len(resp.ResourcePolicies) > 0 {
		return true, nil
	}
	return false, nil
}

type args struct {
	local      bool
	kubeconfig *string
	configFile *string
}

func parseArgs() *args {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to kubectl config")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to kubeconfig file")
	}
	local := flag.Bool("local", false, "use this flag when running locally (outside of cluster to use local kube config")
	configFile := flag.String("config-file", "/etc/disk-manger/config.yaml", "path to yaml file with disk-manager config")
	flag.Parse()
	return &args{*local, kubeconfig, configFile}
}
