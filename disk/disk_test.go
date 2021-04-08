package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/broadinstitute/disk-manager/config"
	"github.com/broadinstitute/disk-manager/logs"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jarcoal/httpmock"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"net/http"
	neturl "net/url"
	"testing"
)

/*
 * Unit tests for Disk Manager. Sources:
 * * https://matt-rickard.com/kubernetes-unit-testing/ (for faking K8s api calls)
 * * https://github.com/jarcoal/httpmock (for faking GCP api calls)
 * Considered but rejected:
 *   https://github.com/googleapis/google-api-go-client/blob/master/testing.md
 *   (httpmock offers a higher level of abstraction)
 */

const gcpComputeURL = "https://compute.googleapis.com/compute/v1"

/* Struct encapsulating a faked GCP API request & response */
type gcpRequest struct {
	method    string
	url       string
	responder httpmock.Responder
	callCount int
}

func TestRun(t *testing.T) {
	cfg := defaultConfig()

	var tests = []struct {
		description string
		k8sObjects  []runtime.Object
		gcpRequests []gcpRequest
	}{
		{description: "no disks"},
		{
			description: "3 disks, 2 zonal, 1 regional; 1 with policy already attached",
			k8sObjects: []runtime.Object{
				fakePVC("pvc-1", "pv-1", map[string]string{cfg.TargetAnnotation: "policy-a"}),
				fakePV("pv-1", "disk-1"),

				fakePVC("pvc-2", "pv-2", map[string]string{cfg.TargetAnnotation: "policy-z"}),
				fakePV("pv-2", "disk-2"),

				fakePVC("pvc-3", "pv-3", map[string]string{cfg.TargetAnnotation: "policy-a"}),
				fakePV("pv-3", "disk-3"),
			},
			gcpRequests: []gcpRequest{
				fakeGetPolicy(cfg, "policy-a", 2), // called for disk 1 and 3
				fakeGetPolicy(cfg, "policy-z", 1), // called for disk 2

				fakeListZonalDisk(cfg, "disk-1", "us-central1-a", []string{}, 1),
				fakeAttachPolicyZonalDisk(cfg, "disk-1", "us-central1-a", "policy-a", 1),

				fakeListRegionalDisk(cfg, "disk-2", "us-central1", []string{}, 1),
				fakeAttachPolicyRegionalDisk(cfg, "disk-2", "us-central1", "policy-z", 1),

				fakeListZonalDisk(cfg, "disk-3", "us-central1-a", []string{"policy-a"}, 1),
				// no attach call -- policy is already attached ^
			},
		},
		{
			description: "2 zonal, in different zones",
			k8sObjects: []runtime.Object{
				fakePVC("pvc-1", "pv-1", map[string]string{cfg.TargetAnnotation: "policy-a"}),
				fakePV("pv-1", "disk-1"),

				fakePVC("pvc-2", "pv-2", map[string]string{cfg.TargetAnnotation: "policy-z"}),
				fakePV("pv-2", "disk-2"),
			},
			gcpRequests: []gcpRequest{
				fakeGetPolicy(cfg, "policy-a", 1), // called for disk 1
				fakeGetPolicy(cfg, "policy-z", 1), // called for disk 2

				fakeListZonalDisk(cfg, "disk-1", "us-central1-a", []string{}, 1),
				fakeAttachPolicyZonalDisk(cfg, "disk-1", "us-central1-a", "policy-a", 1),

				fakeListZonalDisk(cfg, "disk-2", "us-central1-f", []string{}, 1),
				fakeAttachPolicyZonalDisk(cfg, "disk-2", "us-central1-f", "policy-z", 1),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			k8s := k8sfake.NewSimpleClientset(test.k8sObjects...)
			gcp, err := fakeGcp()
			if err != nil {
				t.Errorf("Error constructing fake GCP client: %v", err)
				return
			}
			registerResponders(test.gcpRequests)
			m := DiskManager{config: cfg, gcp: gcp, k8s: k8s}

			// test
			err = m.Run()
			if err != nil {
				t.Errorf("Unexpected error: %s", err)
				return
			}

			err = verifyCallCounts(test.gcpRequests)
			if err != nil {
				t.Error(err)
				return
			}

			// cleanup
			httpmock.DeactivateAndReset()
		})
	}
}

func TestGetDisks(t *testing.T) {
	cfg := defaultConfig()

	var tests = []struct {
		description string
		expected    []diskInfo
		k8sObjects  []runtime.Object
	}{
		{description: "no disks", expected: make([]diskInfo, 0), k8sObjects: nil},
		{
			description: "2 disks",
			expected: []diskInfo{
				{"disk-1", "policy-a"},
				{"disk-2", "policy-z"},
			},
			k8sObjects: []runtime.Object{
				fakePVC("pvc-1", "pv-1", map[string]string{cfg.TargetAnnotation: "policy-a"}),
				fakePV("pv-1", "disk-1"),
				fakePVC("pvc-2", "pv-2", map[string]string{cfg.TargetAnnotation: "policy-z"}),
				fakePV("pv-2", "disk-2"),
			},
		},
		{
			description: "2 disks, 1 without annotation",
			expected: []diskInfo{
				{"disk-2", "policy-a"},
			},
			k8sObjects: []runtime.Object{
				fakePVC("pvc-1", "pv-1", map[string]string{}),
				fakePV("pv-1", "disk-1"),
				fakePVC("pvc-2", "pv-2", map[string]string{cfg.TargetAnnotation: "policy-a"}),
				fakePV("pv-2", "disk-2"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			k8s := k8sfake.NewSimpleClientset(test.k8sObjects...)
			m := DiskManager{config: cfg, gcp: nil, k8s: k8s}
			actual, err := m.searchForDisks()
			if err != nil {
				t.Errorf("Unexpected error: %s", err)
				return
			}
			if diff := cmp.Diff(actual, test.expected, cmpopts.IgnoreUnexported(diskInfo{})); diff != "" {
				t.Errorf("%T differ (-got, +want): %s", test.expected, diff)
				return
			}
		})
	}
}

func TestLastComponentOfURL(t *testing.T) {
	var tests = []struct {
		description string
		url         string
		expected    string
		expectError bool
	}{
		{description: "no path", url: "http://foo.com", expected: "", expectError: true},
		{description: "simple", url: "http://foo.com/path", expected: "path", expectError: false},
		{description: "multiple components", url: "http://foo.com/a/b/c", expected: "c", expectError: false},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			actual, err := lastComponentFromURL(test.url)
			if test.expectError {
				if err == nil {
					t.Errorf("Expected error for %q, but err was nil", test.url)
					return
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected errror for %q: %v", test.url, err)
				}
			}

			if diff := cmp.Diff(actual, test.expected); diff != "" {
				t.Errorf("%T differ (-got, +want): %s", test.expected, diff)
				return
			}
		})
	}
}

/* Default config for all tests */
func defaultConfig() *config.Config {
	return &config.Config{
		TargetAnnotation: "bio.terra.testing/snapshot-policy",
		GoogleProject:    "fake-project",
		Region:           "us-central1",
	}
}

/* Return a GCP client with http requests set up to be intercepted by httpmock.
 * Don't forget to call httpmock.DeactivateAndReset() when you're done!
 */
func fakeGcp() (*compute.Service, error) {
	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	return compute.NewService(context.Background(), option.WithoutAuthentication(), option.WithHTTPClient(client))
}

/* Configure httpmock to respond to a pre-defined set of requests */
func registerResponders(requests []gcpRequest) {
	for _, request := range requests {
		httpmock.RegisterResponder(request.method, request.url, request.responder)
	}
}

/* Verify all expected httpmock requests were made */
func verifyCallCounts(requests []gcpRequest) error {
	counts := httpmock.GetCallCountInfo()
	for _, request := range requests {
		key := fmt.Sprintf("%s %s", request.method, request.url)
		if counts[key] != request.callCount {
			return fmt.Errorf("%s: %d calls expected, %d received", key, request.callCount, counts[key])
		}
	}
	return nil
}

/* Helper functions for generating fake GCP API responses */
func fakeGetPolicy(cfg *config.Config, name string, callCount int) gcpRequest {
	url := fakePolicyLink(cfg.GoogleProject, cfg.Region, name)

	policy := &compute.ResourcePolicy{
		Name:     name,
		SelfLink: url,
	}

	return fakeGetRequest(url, 200, policy, callCount)
}

/* Fake an aggregatedList call for a zonal disk
 * https://cloud.google.com/compute/docs/reference/rest/v1/disks/aggregatedList
 */
func fakeListZonalDisk(cfg *config.Config, name string, zone string, policies []string, callCount int) gcpRequest {
	scope := fmt.Sprintf("zones/%s", zone)
	disk := fakeZonalDisk(cfg, name, zone, policies)
	return fakeDiskAggregatedListRequest(cfg, scope, disk, callCount)
}

/* Fake an aggregatedList call for a regional disk
 * https://cloud.google.com/compute/docs/reference/rest/v1/disks/aggregatedList
 */
func fakeListRegionalDisk(cfg *config.Config, name string, region string, policies []string, callCount int) gcpRequest {
	scope := fmt.Sprintf("regions/%s", region)
	disk := fakeRegionalDisk(cfg, name, region, policies)
	return fakeDiskAggregatedListRequest(cfg, scope, disk, callCount)
}

func fakeDiskAggregatedListRequest(cfg *config.Config, scope string, disk *compute.Disk, callCount int) gcpRequest {
	filter := neturl.QueryEscape(fmt.Sprintf("name = %s", disk.Name))

	// Unfortunately httpmock doesn't support partial query string matches, so we have
	// to include prettyPrint/json parameters :'(
	query := fmt.Sprintf("alt=json&filter=%s&prettyPrint=false", filter)
	url := fmt.Sprintf("%s/projects/%s/aggregated/disks?%s", gcpComputeURL, cfg.GoogleProject, query)

	logs.Info.Printf("URL: %s", url)
	response := &compute.DiskAggregatedList{
		Items:  map[string]compute.DisksScopedList{
			scope: {Disks: []*compute.Disk{disk}},
		},
	}

	return fakeGetRequest(url, 200, response, callCount)
}

func fakeZonalDisk(cfg *config.Config, name string, zone string, policies []string) *compute.Disk {
	return &compute.Disk{
		Name:             name,
		ResourcePolicies: fakePolicyLinks(cfg.GoogleProject, cfg.Region, policies...),
		Zone:             fakeZoneLink(cfg.GoogleProject, zone),
	}
}

func fakeRegionalDisk(cfg *config.Config, name string, region string, policies []string) *compute.Disk {
	return &compute.Disk{
		Name:             name,
		ResourcePolicies: fakePolicyLinks(cfg.GoogleProject, cfg.Region, policies...),
		Region:           fakeRegionLink(cfg.GoogleProject, region),
	}
}

/* Given a project name, region name, and arbitrary number of policy names,
   return a slice of fake policy links.
*/
func fakePolicyLinks(project string, region string, policyNames ...string) []string {
	links := make([]string, len(policyNames))
	for i, policyName := range policyNames {
		links[i] = fakePolicyLink(project, region, policyName)
	}
	return links
}

/* Given a project name, region name, and policy name, return a fake policy link.
   eg. [https://www.googleapis.com/compute/v1/projects/broad-dsde-dev/regions/us-central1/resourcePolicies/my-policy]
*/
func fakePolicyLink(project string, region string, policyName string) string {
	return fmt.Sprintf("%s/projects/%s/regions/%s/resourcePolicies/%s", gcpComputeURL, project, region, policyName)
}

/* Given a project and zone name, return a fake zone link.
   eg. https://www.googleapis.com/compute/v1/projects/broad-dsde-dev/zones/us-central1-f
*/
func fakeZoneLink(project string, zone string) string {
	return fmt.Sprintf("%s/projects/%s/zones/%s", gcpComputeURL, project, zone)
}

/* Given a project and region name, return a fake region link.
   eg. https://www.googleapis.com/compute/v1/projects/broad-dsde-dev/regions/us-central1
*/
func fakeRegionLink(project string, region string) string {
	return fmt.Sprintf("%s/projects/%s/regions/%s", gcpComputeURL, project, region)
}

func fakeAttachPolicyZonalDisk(cfg *config.Config, diskName string, zone string, policyName string, callCount int) gcpRequest {
	url := fmt.Sprintf("%s/projects/%s/zones/%s/disks/%s/addResourcePolicies", gcpComputeURL, cfg.GoogleProject, zone, diskName)

	expectedRequestBody := compute.DisksAddResourcePoliciesRequest{
		ResourcePolicies: fakePolicyLinks(cfg.GoogleProject, cfg.Region, policyName),
	}
	responseBody := compute.DisksAddResourcePoliciesCall{}

	return fakePostRequest(url, expectedRequestBody, 201, responseBody, callCount)
}

func fakeAttachPolicyRegionalDisk(cfg *config.Config, diskName string, region string, policyName string, callCount int) gcpRequest {
	url := fmt.Sprintf("%s/projects/%s/regions/%s/disks/%s/addResourcePolicies", gcpComputeURL, cfg.GoogleProject, region, diskName)

	expectedRequestBody := compute.RegionDisksAddResourcePoliciesRequest{
		ResourcePolicies: fakePolicyLinks(cfg.GoogleProject, cfg.Region, policyName),
	}
	responseBody := compute.RegionDisksAddResourcePoliciesCall{}

	return fakePostRequest(url, expectedRequestBody, 201, responseBody, callCount)
}

func fakeGetRequest(url string, status int, responseBody interface{}, callCount int) gcpRequest {
	responder := httpmock.NewJsonResponderOrPanic(status, responseBody)
	return gcpRequest{ method: "GET", url: url, responder: responder, callCount: callCount}
}

/* Prepare a fake post request with a responder that validates the request body matches the expectedRequestBody parameter */
func fakePostRequest(url string, expectedRequestBody interface{}, status int, responseBody interface{}, callCount int) gcpRequest {
	responder := func(req *http.Request) (*http.Response, error) {
		// We need to compare the _expected_ request body with the _actual_ request body,
		// to make sure we're sending the right API calls to GCP.
		//
		// Since the _actual_ request body is passed to us as a JSON string, but callers of this method
		// should pass in the _expected_ request body as a GCP client struct like
		// `compute.RegionDisksAddResourcePoliciesRequest`, we convert both to map[string]interface{} and compare
		// then with cmp.diff(). To do this, we marshal the expected struct to JSON and then unmarshal it back to
		// map[string]interface{}.
		//
		// A Go expert might be able to do something fancy with reflection, in the mean time this gets the job done :)
		var expected, actual map[string]interface{}

		expectedBytes, err := json.Marshal(expectedRequestBody)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(expectedBytes, &expected); err != nil {
			return nil, err
		}

		actualBytes, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(actualBytes, &actual); err != nil {
			return nil, err
		}

		if diff := cmp.Diff(expected, actual); diff != "" {
			return nil, fmt.Errorf("POST %s\n\t%T differ (-got, +want):\n%s", url, expectedRequestBody, diff)
		}

		return httpmock.NewJsonResponse(status, responseBody)
	}

	return gcpRequest{method: "POST", url: url, responder: responder, callCount: callCount}
}

/* Helper functions for generating fake K8s API objects */
func fakePVC(name string, volumeName string, annotations map[string]string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: volumeName,
		},
	}
}

func fakePV(name string, gceDiskName string) *v1.PersistentVolume {
	pv := v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.PersistentVolumeSpec{},
	}
	pv.Spec.GCEPersistentDisk = &v1.GCEPersistentDiskVolumeSource{
		PDName: gceDiskName,
	}
	return &pv
}
