package main

import (
    //    "context"
    "flag"
    "fmt"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/fields"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/util/homedir"
    "log"
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

    // create the clientset
    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        panic(err.Error())
    }

    pvcs, err := clientset.CoreV1().PersistentVolumeClaims("").List(metav1.ListOptions{})

    if err != nil {
        panic(err.Error())
    }

    // TODO - pagination needed?
    for _, pvc := range pvcs.Items {
        if policy, ok := pvc.Annotations["bio.terra/snapshot-policy"]; ok {
            fmt.Printf("%s: %s\n", pvc.Name, policy)
            fmt.Printf("%s\n", pvc.Spec.VolumeName)

            selector := fields.OneTermEqualSelector("metadata.name", pvc.Spec.VolumeName)
            listOptions := metav1.ListOptions{ FieldSelector: selector.String()}
            pvs, err := clientset.CoreV1().PersistentVolumes().List(listOptions)
            if err != nil {
                panic(err.Error())
            }
            if len(pvs.Items) != 1 {
                log.Panicf("only one PV should match query, got %d", len(pvs.Items))
            }
            pd := pvs.Items[0]
            gcpDiskName := pd.Spec.GCEPersistentDisk.PDName
            fmt.Println(gcpDiskName)
        }
    }
/*
    v1.PersistentVolumeClaim{TypeMeta:v1.TypeMeta{Kind:"", APIVersion:""}, ObjectMeta:v1.ObjectMeta{Name:"datadir-mongodb-2", GenerateName:"", Namespace:"terra-dev", SelfLink:"/api/v1/namespaces/terra-dev/persistentvolumeclaims/datadir-mongodb-2", UID:"1cf1fb2b-3ad5-462a-8460-e9fb974cebe2", ResourceVersion:"208402350", Generation:0, CreationTimestamp:v1.Time{Time:time.Time{wall:0x0, ext:63747796819, loc:(*time.Location)(0x28299a0)}}, DeletionTimestamp:(*v1.Time)(nil), DeletionGracePeriodSeconds:(*int64)(nil), Labels:map[string]string{"app.kubernetes.io/component":"mongodb", "app.kubernetes.io/instance":"mongodb", "app.kubernetes.io/name":"bitnami"}, Annotations:map[string]string{"bio.terra/snapshot-policy":"terra-snapshot-policy", "pv.kubernetes.io/bind-completed":"yes", "pv.kubernetes.io/bound-by-controller":"yes", "volume.beta.kubernetes.io/storage-provisioner":"kubernetes.io/gce-pd"}, OwnerReferences:[]v1.OwnerReference(nil), Finalizers:[]string{"kubernetes.io/pvc-protection"}, ClusterName:"", ManagedFields:[]v1.ManagedFieldsEntry(nil)}, Spec:v1.PersistentVolumeClaimSpec{AccessModes:[]v1.PersistentVolumeAccessMode{"ReadWriteOnce"}, Selector:(*v1.LabelSelector)(nil), Resources:v1.ResourceRequirements{Limits:v1.ResourceList(nil), Requests:v1.ResourceList{"storage":resource.Quantity{i:resource.int64Amount{value:53687091200, scale:0}, d:resource.infDecAmount{Dec:(*inf.Dec)(nil)}, s:"50Gi", Format:"BinarySI"}}}, VolumeName:"pvc-1cf1fb2b-3ad5-462a-8460-e9fb974cebe2", StorageClassName:(*string)(0xc0003ec250), VolumeMode:(*v1.PersistentVolumeMode)(0xc0003ec260), DataSource:(*v1.TypedLocalObjectReference)(nil)}, Status:v1.PersistentVolumeClaimStatus{Phase:"Bound", AccessModes:[]v1.PersistentVolumeAccessMode{"ReadWriteOnce"}, Capacity:v1.ResourceList{"storage":resource.Quantity{i:resource.int64Amount{value:53687091200, scale:0}, d:resource.infDecAmount{Dec:(*inf.Dec)(nil)}, s:"50Gi", Format:"BinarySI"}}, Conditions:[]v1.PersistentVolumeClaimCondition(nil)}}%
 */
        //pods, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{})
        //if err != nil {
        //    panic(err.Error())
        //}
        //fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))
        //
        //// Examples for error handling:
        //// - Use helper functions like e.g. errors.IsNotFound()
        //// - And/or cast to StatusError and use its properties like e.g. ErrStatus.Message
        //namespace := "terra-dev"
        //pod := "example-xxxxx"
        //_, err = clientset.CoreV1().Pods(namespace).Get(pod, metav1.GetOptions{})
        //if errors.IsNotFound(err) {
        //    fmt.Printf("Pod %s in namespace %s not found\n", pod, namespace)
        //} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
        //    fmt.Printf("Error getting pod %s in namespace %s: %v\n",
        //        pod, namespace, statusError.ErrStatus.Message)
        //} else if err != nil {
        //    panic(err.Error())
        //} else {
        //    fmt.Printf("Found pod %s in namespace %s\n", pod, namespace)
        //}
        //
        //time.Sleep(10 * time.Second)
    // }
}