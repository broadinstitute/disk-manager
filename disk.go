package main

import (
	"fmt"
	"github.com/broadinstitute/disk-manager/client"
	"github.com/broadinstitute/disk-manager/config"
	"github.com/broadinstitute/disk-manager/logs"
	"google.golang.org/api/compute/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type diskManager struct {
	config *config.Config       // DiskManager config
	gcp    *compute.Service     // GCP Compute API client
	k8s    kubernetes.Interface // K8s API client
}

type diskInfo struct {
	name   string
	policy string
}

/* Construct a new DiskManager */
func newDiskManager(args *args) (*diskManager, error) {
	cfg, err := config.Read(*args.configFile)
	if err != nil {
		return nil, fmt.Errorf("Error building config: %v\n", err)
	}

	logs.Info.Printf("Building clients...")
	clients, err := client.Build(args.local, args.kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("Error building clients: %v, exiting\n", err)
	}

	k8s := clients.GetK8s()
	gcp := clients.GetGCP()

	return &diskManager{cfg, gcp, k8s}, nil
}

/* Add configured snapshot policies to all disks with the configured annotation */
func (m *diskManager) addPoliciesToDisks() error {
	logs.Info.Println("Searching for persistent disks...")
	disks, err := m.getDisks()
	if err != nil {
		return fmt.Errorf("Error retrieving persistent disks: %v\n", err)
	}

	errs := 0
	for _, disk := range disks {
		if err := m.addPolicy(disk); err != nil {
			logs.Error.Printf("Error adding policy %s to disk %s: %v\n", disk.policy, disk.name, err)
			errs++
		}
	}

	if errs > 0 {
		return fmt.Errorf("Encountered %d error(s) adding snapshot policies to disks\n", errs)
	}

	logs.Info.Println("Finished updating snapshot policies")
	return nil
}

/* Search K8s for PersistentVolumeClaims with the snapshot policy annotation */
func (m *diskManager) getDisks() ([]diskInfo, error) {
	disks := make([]diskInfo, 0)

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
func (m *diskManager) addPolicy(info diskInfo) error {
	// TODO only perform this api call if policyName is different
	policy, err := m.getPolicy(info.policy)
	if err != nil {
		return fmt.Errorf("Error retrieving snapshot policy %s for disk %s: %v\n", info.policy, info.name, err)
	}

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

	// Attach policy
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
func (m *diskManager) findDisk(name string) (*compute.Disk, bool, error) {
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
	logs.Error.Printf("Could not find disk %s in region %s: %v\n", name, m.config.Region, err2)

	return nil, false, fmt.Errorf("Could not find disk %s in configured region or zone\n", name)
}

/* Retrieve a resource policy object via the GCP API */
func (m *diskManager) getPolicy(name string) (*compute.ResourcePolicy, error) {
	return m.gcp.ResourcePolicies.Get(m.config.GoogleProject, m.config.Region, name).Do()
}

/* Retrieve a zonal disk object via the GCP API */
func (m *diskManager) getZonalDisk(name string) (*compute.Disk, error) {
	return m.gcp.Disks.Get(m.config.GoogleProject, m.config.Zone, name).Do()
}

/* Retrieve a regional disk object via the GCP API */
func (m *diskManager) getRegionalDisk(name string) (*compute.Disk, error) {
	return m.gcp.RegionDisks.Get(m.config.GoogleProject, m.config.Region, name).Do()
}

/* Attach a policy to a zonal disk object via the GCP API */
func (m *diskManager) addPolicyToZonalDisk(diskName string, policy *compute.ResourcePolicy) error {
	addPolicyRequest := &compute.DisksAddResourcePoliciesRequest{
		ResourcePolicies: []string{policy.SelfLink},
	}
	_, err := m.gcp.Disks.AddResourcePolicies(m.config.GoogleProject, m.config.Zone, diskName, addPolicyRequest).Do()
	return err
}

/* Attach a policy to a regional disk object via the GCP API */
func (m *diskManager) addPolicyToRegionalDisk(diskName string, policy *compute.ResourcePolicy) error {
	addPolicyRequest := &compute.RegionDisksAddResourcePoliciesRequest{
		ResourcePolicies: []string{policy.SelfLink},
	}
	_, err := m.gcp.RegionDisks.AddResourcePolicies(m.config.GoogleProject, m.config.Region, diskName, addPolicyRequest).Do()
	return err
}
