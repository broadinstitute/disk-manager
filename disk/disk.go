package disk

import (
	"fmt"
	"github.com/broadinstitute/disk-manager/client"
	"github.com/broadinstitute/disk-manager/config"
	"github.com/broadinstitute/disk-manager/logs"
	"google.golang.org/api/compute/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	neturl "net/url"
	"strings"
)

type DiskManager struct {
	config *config.Config       // DiskManager config
	gcp    *compute.Service     // GCP Compute API client
	k8s    kubernetes.Interface // K8s API client
}

type diskInfo struct {
	name   string
	policy string
}

/* Construct a new DiskManager */
func NewDiskManager(cfg *config.Config, clients *client.Clients) (*DiskManager, error) {
	k8s := clients.GetK8s()
	gcp := clients.GetGCP()

	return &DiskManager{cfg, gcp, k8s}, nil
}

/*
 * Main method for disk manager.
 * Add snapshot policies to all persistent disks with the configured annotation.
 */
func (m *DiskManager) Run() error {
	disks, err := m.searchForDisks()
	if err != nil {
		return fmt.Errorf("Error retrieving persistent disks: %v\n", err)
	}

	return m.addPoliciesToDisks(disks)
}

/* Search K8s for PersistentVolumeClaims with the snapshot policy annotation */
func (m *DiskManager) searchForDisks() ([]diskInfo, error) {
	disks := make([]diskInfo, 0)

	logs.Info.Println("Searching GKE for persistent disks...")

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

/* Add snapshot policies to disks */
func (m *DiskManager) addPoliciesToDisks(disks []diskInfo) error {
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

/* Add the configured resource policy to the target disk */
func (m *DiskManager) addPolicy(info diskInfo) error {
	// TODO only perform this api call if policyName is different
	policy, err := m.getPolicy(info.policy)
	if err != nil {
		return fmt.Errorf("Error retrieving snapshot policy %s for disk %s: %v\n", info.policy, info.name, err)
	}

	disk, err := m.findDisk(info.name)

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

	if isRegional(disk) {
		logs.Info.Printf("Disk %s appears to be regional: %s", disk.Name, disk.Region)
		err = m.addPolicyToRegionalDisk(disk, policy)
	} else {
		logs.Info.Printf("Disk %s appears to be zonal: %s", disk.Name, disk.Zone)
		err = m.addPolicyToZonalDisk(disk, policy)
	}
	if err != nil {
		return fmt.Errorf("Error adding snapshot policy %s to disk %s: %v\n", info.policy, info.name, err)
	}

	logs.Info.Printf("Added policy %s to disk %s\n", info.policy, info.name)
	return nil
}

/* Retrieve a regional or zonal disk object via the GCP API.
   Returns the disk, and an error. Callers can determine whether the disk is regional or zonal by
   checking the Zone attribute (empty for regional disk) or Region attribute (empty for zonal disk).
*/
func (m *DiskManager) findDisk(name string) (*compute.Disk, error) {
	aggregatedList, err := m.listDisksWithName(name)
	if err != nil {
		return nil, err
	}

	disks := make([]*compute.Disk, 0)
	for _, list := range aggregatedList.Items {
		if len(list.Disks) > 0 {
			disks = append(disks, list.Disks...)
		}
	}

	if len(disks) != 1 {
		return nil, fmt.Errorf("Expected exactly one disk matching name %s, got %d:\n%v\n", name, len(disks), disks)
	}

	return disks[0], nil
}

/* Retrieve a resource policy object via the GCP API */
func (m *DiskManager) getPolicy(name string) (*compute.ResourcePolicy, error) {
	return m.gcp.ResourcePolicies.Get(m.config.GoogleProject, m.config.Region, name).Do()
}

/* Lists disks with the given name via the GCP API */
func (m *DiskManager) listDisksWithName(name string) (*compute.DiskAggregatedList, error) {
	filter := fmt.Sprintf("name = %s", name)
	return m.gcp.Disks.AggregatedList(m.config.GoogleProject).Filter(filter).Do()
}

/* Attach a policy to a zonal disk object via the GCP API */
func (m *DiskManager) addPolicyToZonalDisk(disk *compute.Disk, policy *compute.ResourcePolicy) error {
	addPolicyRequest := &compute.DisksAddResourcePoliciesRequest{
		ResourcePolicies: []string{policy.SelfLink},
	}
	zone, err := zoneName(disk)
	if err != nil {
		return err
	}
	_, err = m.gcp.Disks.AddResourcePolicies(m.config.GoogleProject, zone, disk.Name, addPolicyRequest).Do()
	return err
}

/* Attach a policy to a regional disk object via the GCP API */
func (m *DiskManager) addPolicyToRegionalDisk(disk *compute.Disk, policy *compute.ResourcePolicy) error {
	addPolicyRequest := &compute.RegionDisksAddResourcePoliciesRequest{
		ResourcePolicies: []string{policy.SelfLink},
	}
	region, err := regionName(disk)
	if err != nil {
		return err
	}
	_, err = m.gcp.RegionDisks.AddResourcePolicies(m.config.GoogleProject, region, disk.Name, addPolicyRequest).Do()
	return err
}

func isRegional(disk *compute.Disk) bool {
	return disk.Region != ""
}

func zoneName(disk *compute.Disk) (string, error) {
	return lastComponentFromURL(disk.Zone)
}

func regionName(disk *compute.Disk) (string, error) {
	return lastComponentFromURL(disk.Region)
}

/* Given a URL string, return the last component of the path. Eg.
 * "https://foo.com/p1/p2/p3?n=2" => "p3"
 */
func lastComponentFromURL(url string) (string, error) {
	parsed, err := neturl.Parse(url)
	if err != nil {
		return "", err
	}

	tokens := strings.Split(parsed.Path, "/")
	if len(tokens) > 0 {
		last := tokens[len(tokens) - 1]
		if last != "" {
			return last, nil
		}
	}

	return "", fmt.Errorf("failed to extract last component from url path: %s", parsed.Path)
}
