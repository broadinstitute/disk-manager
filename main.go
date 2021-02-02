package main

import (
    //    "context"
    "flag"
    "fmt"
    "google.golang.org/api/compute/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/fields"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/util/homedir"
    "log"
    "net/http"
    "path/filepath"
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
    // Kubernets POC starts here
    var kubeconfig *string
    if home := homedir.HomeDir(); home != "" {
        kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
    } else {
        kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
    }
    flag.Parse()

    // use the current context in kubeconfig
    config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
    if err != nil {
        panic(err.Error())
    }

    // create the k8s client (technically a "clientset")
    k8s, err := kubernetes.NewForConfig(config)
    if err != nil {
        panic(err.Error())
    }

    pvcs, err := k8s.CoreV1().PersistentVolumeClaims("").List(metav1.ListOptions{})

    if err != nil {
        panic(err.Error())
    }

    // TODO - pagination needed?
    for _, pvc := range pvcs.Items {
        if policy, ok := pvc.Annotations["bio.terra/snapshot-policy"]; ok {
            fmt.Printf("pvc name: %s\n", pvc.Name)
            fmt.Printf("snapshot policy: %s\n", policy)
            fmt.Printf("volume name: %s\n", pvc.Spec.VolumeName)

            selector := fields.OneTermEqualSelector("metadata.name", pvc.Spec.VolumeName)
            listOptions := metav1.ListOptions{ FieldSelector: selector.String()}
            pvs, err := k8s.CoreV1().PersistentVolumes().List(listOptions)
            if err != nil {
                panic(err.Error())
            }
            if len(pvs.Items) != 1 {
                log.Panicf("Exactly one PV should match query, got %d", len(pvs.Items))
            }
            pd := pvs.Items[0]
            gcpDiskName := pd.Spec.GCEPersistentDisk.PDName
            fmt.Printf("gcp disk name: %s\n", gcpDiskName)
            fmt.Println()
        }
    }

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