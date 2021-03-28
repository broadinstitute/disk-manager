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
	args := parseArgs()

	m, err := newDiskManager(args)
	if err != nil {
		logs.Error.Fatal(err)
	}

	logs.Info.Println("Searching for persistent disks...")
	disks, err := m.getDisks()
	if err != nil {
		logs.Error.Fatalf("Error retrieving persistent disks: %v\n", err)
	}

	errs := 0
	for _, disk := range disks {
		if err := m.addPolicy(disk); err != nil {
			logs.Error.Printf("Error adding policy %s to disk %s: %v\n", disk.policy, disk.name, err)
			errs++
		}
	}

	if errs > 0 {
		logs.Error.Fatalf("Encountered %d error(s) adding snapshot policies to disks\n", errs)
	}
	logs.Info.Println("Finished updating snapshot policies")
}

type diskManager struct {
	config *config.Config     // DiskManager config
	gcp *compute.Service      // GCP Compute API client
	ctx context.Context       // context for GCP API client
	k8s *kubernetes.Clientset // K8s API client
}

/* Construct a new DiskManager */
func newDiskManager(args *args) (*diskManager, error) {
	config, err := config.Read(*args.configFile)
	if err != nil {
		return nil, fmt.Errorf("Error building config: %v\n", err)
	}

	logs.Info.Printf("Building clients...")
	clients, err := client.Build(args.local, args.kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("Error building clients: %v, exiting\n", err)
	}

	k8s := clients.GetK8s()
	ctx := context.Background()
	gcp := clients.GetGCP()

	return &diskManager{config, gcp, ctx, k8s}, nil
}

type diskInfo struct {
	name   string
	policy string
}

/* Search K8s for PersistentVolumeClaims with the snapshot policy annotation */
func (m diskManager) getDisks() ([]diskInfo, error) {
	var disks []diskInfo

	// get persistent volume claims
	pvcs, err := m.k8s.CoreV1().PersistentVolumeClaims("").List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("Error retrieving persistent volume claims: %v\n", err)
	}
	for _, pvc := range pvcs.Items {
		if policy, ok := pvc.Annotations[m.config.TargetAnnotation]; ok {
			// retrieve associated persistent volume for each claim
			pv, err := m.k8s.CoreV1().PersistentVolumes().Get(pvc.Spec.VolumeName, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("Error retrieving persistent volume: %s, %v\n", pvc.Spec.VolumeName, err)
			}
			diskName := pv.Spec.GCEPersistentDisk.PDName
			logs.Info.Printf("found PersistentVolume: %q with disk: %q", pvc.GetName(), diskName)
			disk := diskInfo{
				name:   diskName,
				policy: policy,
			}
			disks = append(disks, disk)
		}
	}
	return disks, nil
}

/* Add the configured resource policy to the target disk */
func (m diskManager) addPolicy(info diskInfo) error {
	// Retrieve the policy from the GCP API
	// TODO only perform this api call if policyName is different
	policy, err := m.getPolicy(info.policy)
	if err != nil {
		return fmt.Errorf("Error retrieving snapshot policy %s for disk %s: %v\n", info.policy, info.name, err)
	}

	// Query GCP for the disk
	disk, regional, err := m.findDisk(info.name)
	if err != nil {
		return err
	}

	// Check to see if any policies are already attached
	if len(disk.ResourcePolicies) > 1 {
		return fmt.Errorf("Disk %s has more than one resource policy, did the GCP API change? %v\n", info.name, disk.ResourcePolicies)
	}
	if len(disk.ResourcePolicies) == 1 {
		if disk.ResourcePolicies[0] == policy.SelfLink {
			logs.Info.Printf("Policy %s is already attached to disk %s, nothing to do\n", info.policy, info.name)
			return nil
		} else {
			return fmt.Errorf("Unexpected policy %s is already attached to disk %s, please detach it manually and re-run\n", disk.ResourcePolicies[0], info.name)
		}
	}

	// Attach policy to disk
	err = nil
	if regional {
		err = m.addPolicyToRegionalDisk(info.name, policy)
	} else {
		err = m.addPolicyToZonalDisk(info.name, policy)
	}
	if err != nil {
		return fmt.Errorf("Error adding snapshot policy %s to disk %s: %v\n", info.policy, info.name, err)
	}

	logs.Info.Printf("Added policy %s to disk %s\n", info.policy, info.name)
	return nil
}

/* Retrieve a regional or zonal disk object via the GCP API.
   Returns the disk, a boolean that is true if the disk is regional, and an error
 */
func (m diskManager) findDisk(name string) (*compute.Disk, bool, error) {
	disk, err1 := m.getZonalDisk(name)
	if err1 == nil {
		logs.Info.Printf("Found disk %s in zone %s\n", name, m.config.Zone)
		return disk, false, nil
	}

	disk, err2 := m.getRegionalDisk(name)
	if err2 == nil {
		logs.Info.Printf("Found disk %s in region %s", name, m.config.Region)
		return disk, true, nil
	}

	logs.Error.Printf("Could not find disk %s in zone %s: %v\n", name, m.config.Zone, err1)
	logs.Error.Printf("Could not find disk %s in region %s: %v\n", name,m.config.Region, err2)

	return nil, false, fmt.Errorf("Could not find disk %s in configured region or zone\n", name)
}

/* Retrieve a resource policy object via the GCP API */
func (m diskManager) getPolicy(name string) (*compute.ResourcePolicy, error) {
	return m.gcp.ResourcePolicies.Get(m.config.GoogleProject, m.config.Region, name).Context(m.ctx).Do()
}

/* Retrieve a zonal disk object via the GCP API */
func (m diskManager) getZonalDisk(name string) (*compute.Disk, error) {
	return m.gcp.Disks.Get(m.config.GoogleProject, m.config.Zone, name).Context(m.ctx).Do()
}

/* Retrieve a regional disk object via the GCP API */
func (m diskManager) getRegionalDisk(name string) (*compute.Disk, error) {
	return m.gcp.RegionDisks.Get(m.config.GoogleProject, m.config.Region, name).Context(m.ctx).Do()
}

/* Attach a policy to a zonal disk object via the GCP API */
func (m diskManager) addPolicyToZonalDisk(diskName string, policy *compute.ResourcePolicy) error {
	addPolicyRequest := &compute.DisksAddResourcePoliciesRequest{
		ResourcePolicies: []string{policy.SelfLink},
	}
	_, err := m.gcp.Disks.AddResourcePolicies(m.config.GoogleProject, m.config.Zone, diskName, addPolicyRequest).Context(m.ctx).Do()
	return err
}

/* Attach a policy to a regional disk object via the GCP API */
func (m diskManager) addPolicyToRegionalDisk(diskName string, policy *compute.ResourcePolicy) error {
	addPolicyRequest := &compute.RegionDisksAddResourcePoliciesRequest{
		ResourcePolicies: []string{policy.SelfLink},
	}
	_, err := m.gcp.RegionDisks.AddResourcePolicies(m.config.GoogleProject, m.config.Region, diskName, addPolicyRequest).Context(m.ctx).Do()
	return err
}

type args struct {
	local      bool
	kubeconfig *string
	configFile *string
}

/* Parse command-line arguments */
func parseArgs() *args {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to kubectl config")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to kubeconfig file")
	}
	local := flag.Bool("local", false, "use this flag when running locally (outside of cluster to use local kube config")
	configFile := flag.String("config-file", "/etc/disk-manager/config.yaml", "path to yaml file with disk-manager config")
	flag.Parse()
	return &args{*local, kubeconfig, configFile}
}
