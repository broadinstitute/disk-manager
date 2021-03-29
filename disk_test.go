package main

import (
	"github.com/broadinstitute/disk-manager/config"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
)

// https://matt-rickard.com/kubernetes-unit-testing/
func TestAddPoliciesToDisks(t *testing.T) {
	config := config.Config{
		TargetAnnotation: "bio.terra.testing/snapshot-policy",
		GoogleProject:    "fake-project",
		Zone:             "us-central1",
		Region:           "us-central1-a",
	}

	testObjects := []runtime.Object{
		pvc("test-pvc-1", "test-pv-1", map[string]string{config.TargetAnnotation: "snapshot-policy-1"}),
		pv("test-pv-1", "gce-disk-1"),
	}

	k8s := fake.NewSimpleClientset(testObjects...)
	m := diskManager{config: &config, gcp: nil, ctx: nil, k8s: k8s}
	disks, err := m.getDisks()
	if err != nil {
		t.Error(err)
	}
	if len(disks) != 1 {
		t.Errorf("Expected 1 disks, got %d", len(disks))
	}
	if disks[0].policy != "snapshot-policy-1" {
		t.Errorf("Expected snapshot policy snapshot-policy-1, got %s", disks[0].policy)
	}
	if disks[0].name != "gce-disk-1" {
		t.Errorf("Expected disk name to be gce-disk1, got %s", disks[0].name)
	}
}

func pvc(name string, volumeName string, annotations map[string]string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: annotations,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: volumeName,
		},
	}
}

func pv(name string, gceDiskName string) *v1.PersistentVolume {
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
