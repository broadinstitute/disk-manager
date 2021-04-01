package client

import (
	"fmt"

	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Build will return a k8s client using local kubectl
// config

// Clients struct containing the GCP and k8s clients used in this tool
type Clients struct {
	gcp *compute.Service
	k8s *kubernetes.Clientset
}

// GetGCP will return a handle to the gcp client generated by the builder
func (c *Clients) GetGCP() *compute.Service {
	return c.gcp
}

// GetK8s will return  a handle to the kubernetes client generated by the builder
func (c *Clients) GetK8s() *kubernetes.Clientset {
	return c.k8s
}

// Build creates the GCP and k8s clients used by this tool
// and returns both packaged in a single struct
func Build(local bool, kubeconfig *string) (*Clients, error) {
	conf, err := buildKubeConfig(local, kubeconfig)
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
		gcp,
		k8s,
	}, nil
}

func buildKubeConfig(local bool, kubeconfig *string) (*restclient.Config, error) {
	if local {
		config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("Error building local k8s config: %v", err)
		}
		return config, nil
	}
	config, err := restclient.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("Error building in cluster k8s config: %v", err)
	}
	return config, nil
}

func buildKubeClient(config *restclient.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(config)
}

func buildGCPClient() (*compute.Service, error) {
	ctx := context.Background()

	c, err := compute.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating compute api client: %v", err)
	}
	return c, nil
}
