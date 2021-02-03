package client

import (
	"flag"
	"fmt"
	"path/filepath"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Build will return a k8s client using local kubectl
// config

// Clients struct containing the GCP and k8s clients used in this tool
type Clients struct {
	GCP *compute.Service
	K8s *kubernetes.Clientset
}

// Build creates the GCP and k8s clients used by this tool
// and returns both packaged in a single struct
func Build() (*Clients, error) {
	conf, err := buildKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("Error building kube client: %v", err)
	}
	k8s, err := buildKubeClient(conf)
	if err != nil {
		return nil, fmt.Errorf("Error building kube client: %v", err)
	}

	gcp, err := buildGCPClient()
	if err != nil {
		return nil, fmt.Errorf("Error building GCP client: %v", err)
	}
	return &Clients{
		GCP: gcp,
		K8s: k8s,
	}, nil
}

func buildKubeConfig() (*restclient.Config, error) {
	var kubeconfig *string

	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to kubectl config")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("Error building k8s config: %v", err)
	}
	return config, nil
}

func buildKubeClient(config *restclient.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(config)
}

func buildGCPClient() (*compute.Service, error) {
	ctx := context.Background()

	gcpClient, err := google.DefaultClient(ctx, compute.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("error authenticating to GCP: %v", err)
	}

	c, err := compute.New(gcpClient)
	if err != nil {
		return nil, fmt.Errorf("error creating compute api client: %v", err)
	}
	return c, nil
}
