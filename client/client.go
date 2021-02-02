package client

import (
	"flag"
	"fmt"
	"path/filepath"

	"k8s.io/client-go/kubernetes"

	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Build will return a k8s client using local kubectl
// config
func Build() (*kubernetes.Clientset, error) {
	conf, err := buildKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("Error building kube client: %v", err)
	}
	client, err := buildKubeClient(conf)
	if err != nil {
		return nil, fmt.Errorf("Error building kube client: %v", err)
	}
	return client, nil
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
